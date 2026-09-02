package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"mime"
	"strconv"
	"strings"

	logsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func isProtobuf(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && (mediaType == "application/x-protobuf" || mediaType == "application/protobuf")
}

func readProto(body io.Reader, message proto.Message) error {
	payload, err := io.ReadAll(io.LimitReader(body, maxBodyBytes+1))
	if err != nil || len(payload) == 0 || len(payload) > maxBodyBytes || proto.Unmarshal(payload, message) != nil {
		return ErrInvalidOTLP
	}
	return nil
}

func (g Gateway) acceptOTLPTracesProtobuf(ctx context.Context, tenant string, body io.Reader) error {
	request := new(tracev1.ExportTraceServiceRequest)
	if err := readProto(body, request); err != nil {
		return err
	}
	return g.acceptTraceRequest(ctx, tenant, request, "otlp_http_protobuf")
}

func (g Gateway) acceptOTLPLogsProtobuf(ctx context.Context, tenant string, body io.Reader) error {
	request := new(logsv1.ExportLogsServiceRequest)
	if err := readProto(body, request); err != nil {
		return err
	}
	return g.acceptLogsRequest(ctx, tenant, request, "otlp_http_protobuf")
}

func (g Gateway) acceptTraceRequest(ctx context.Context, tenant string, request *tracev1.ExportTraceServiceRequest, source string) error {
	events := make([]Event, 0)
	for _, resourceSpans := range request.GetResourceSpans() {
		resource := protoResourceAttributes(resourceSpans.GetResource())
		for _, scopeSpans := range resourceSpans.GetScopeSpans() {
			for _, span := range scopeSpans.GetSpans() {
				if len(events) >= maxSpans {
					return ErrInvalidOTLP
				}
				attributes := mergeProtoAttributes(resource, span.GetAttributes())
				attributes["telemetry.source"] = source
				attributes["span.kind"] = span.GetKind().String()
				if span.GetStatus() != nil {
					attributes["span.status"] = span.GetStatus().GetCode().String()
				}
				event := Event{
					EventID:      hex.EncodeToString(span.GetTraceId()) + ":" + hex.EncodeToString(span.GetSpanId()),
					Signal:       "trace",
					TraceID:      hex.EncodeToString(span.GetTraceId()),
					SpanID:       hex.EncodeToString(span.GetSpanId()),
					ParentSpanID: hex.EncodeToString(span.GetParentSpanId()),
					Name:         span.GetName(),
					Attributes:   attributes,
				}
				if !valid(event) {
					return ErrInvalidOTLP
				}
				events = append(events, event)
			}
		}
	}
	if len(events) == 0 {
		return ErrInvalidOTLP
	}
	return g.acceptEvents(ctx, tenant, events)
}

func (g Gateway) acceptLogsRequest(ctx context.Context, tenant string, request *logsv1.ExportLogsServiceRequest, source string) error {
	events := make([]Event, 0)
	for _, resourceLogs := range request.GetResourceLogs() {
		resource := protoResourceAttributes(resourceLogs.GetResource())
		for _, scopeLogs := range resourceLogs.GetScopeLogs() {
			for _, record := range scopeLogs.GetLogRecords() {
				if len(events) >= maxSpans {
					return ErrInvalidOTLP
				}
				attributes := mergeProtoAttributes(resource, record.GetAttributes())
				attributes["telemetry.source"] = source
				if severity := record.GetSeverityText(); severity != "" {
					attributes["log.severity"] = severity
				}
				if body, ok := protoScalar(record.GetBody()); ok {
					attributes["log.body"] = body
				}
				identity := hex.EncodeToString(record.GetTraceId()) + ":" + hex.EncodeToString(record.GetSpanId()) + ":" + strconv.FormatUint(record.GetTimeUnixNano(), 10) + ":" + strconv.Itoa(len(events)+1)
				sum := sha256.Sum256([]byte(identity))
				event := Event{
					EventID:    "log-" + hex.EncodeToString(sum[:8]),
					Signal:     "log",
					TraceID:    hex.EncodeToString(record.GetTraceId()),
					SpanID:     hex.EncodeToString(record.GetSpanId()),
					Name:       "log",
					Attributes: attributes,
				}
				if !valid(event) {
					return ErrInvalidOTLP
				}
				events = append(events, event)
			}
		}
	}
	if len(events) == 0 {
		return ErrInvalidOTLP
	}
	return g.acceptEvents(ctx, tenant, events)
}

func protoResourceAttributes(resource *resourcev1.Resource) map[string]string {
	if resource == nil {
		return map[string]string{}
	}
	return mergeProtoAttributes(nil, resource.GetAttributes())
}

func mergeProtoAttributes(base map[string]string, attributes []*commonv1.KeyValue) map[string]string {
	result := make(map[string]string, len(base)+len(attributes))
	for key, value := range base {
		result[key] = value
	}
	for _, attribute := range attributes {
		if attribute == nil || attribute.GetKey() == "" {
			continue
		}
		if value, ok := protoScalar(attribute.GetValue()); ok {
			result[attribute.GetKey()] = value
		}
	}
	return result
}

func protoScalar(value *commonv1.AnyValue) (string, bool) {
	if value == nil {
		return "", false
	}
	switch typed := value.Value.(type) {
	case *commonv1.AnyValue_StringValue:
		return typed.StringValue, true
	case *commonv1.AnyValue_BoolValue:
		return strconv.FormatBool(typed.BoolValue), true
	case *commonv1.AnyValue_IntValue:
		return strconv.FormatInt(typed.IntValue, 10), true
	case *commonv1.AnyValue_DoubleValue:
		return strconv.FormatFloat(typed.DoubleValue, 'g', -1, 64), true
	default:
		return "", false
	}
}

type GRPCServer struct {
	tracev1.UnimplementedTraceServiceServer
	logsv1.UnimplementedLogsServiceServer
	Gateway Gateway
}

func RegisterGRPC(server *grpc.Server, gateway Gateway) {
	service := &GRPCServer{Gateway: gateway}
	tracev1.RegisterTraceServiceServer(server, service)
	logsv1.RegisterLogsServiceServer(server, &logsGRPCServer{parent: service})
}

func (s *GRPCServer) Export(ctx context.Context, request *tracev1.ExportTraceServiceRequest) (*tracev1.ExportTraceServiceResponse, error) {
	tenant, err := s.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.Gateway.acceptTraceRequest(ctx, tenant, request, "otlp_grpc"); err != nil {
		return nil, grpcError(err)
	}
	return &tracev1.ExportTraceServiceResponse{}, nil
}

func (s *GRPCServer) ExportLogs(ctx context.Context, request *logsv1.ExportLogsServiceRequest) (*logsv1.ExportLogsServiceResponse, error) {
	tenant, err := s.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.Gateway.acceptLogsRequest(ctx, tenant, request, "otlp_grpc"); err != nil {
		return nil, grpcError(err)
	}
	return &logsv1.ExportLogsServiceResponse{}, nil
}

// Export implements both generated services. Go cannot overload method names,
// so the logs adapter is exposed through a small wrapper registered separately.
type logsGRPCServer struct {
	logsv1.UnimplementedLogsServiceServer
	parent *GRPCServer
}

func (s *logsGRPCServer) Export(ctx context.Context, request *logsv1.ExportLogsServiceRequest) (*logsv1.ExportLogsServiceResponse, error) {
	return s.parent.ExportLogs(ctx, request)
}

func (s *GRPCServer) authenticate(ctx context.Context) (string, error) {
	values, _ := metadata.FromIncomingContext(ctx)
	key := firstMetadata(values, "x-paop-api-key")
	tenant, ok, err := s.Gateway.Authenticator.Tenant(ctx, key)
	if err != nil {
		return "", status.Error(codes.Unavailable, "gateway unavailable")
	}
	if !ok {
		return "", status.Error(codes.Unauthenticated, "unauthorized")
	}
	return tenant, nil
}

func firstMetadata(values metadata.MD, key string) string {
	for _, value := range values.Get(strings.ToLower(key)) {
		if value != "" {
			return value
		}
	}
	return ""
}

func grpcError(err error) error {
	if err == ErrInvalidOTLP {
		return status.Error(codes.InvalidArgument, "invalid telemetry envelope")
	}
	return status.Error(codes.Unavailable, "durable publish unavailable")
}
