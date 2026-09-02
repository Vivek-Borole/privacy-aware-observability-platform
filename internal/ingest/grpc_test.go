package ingest

import (
	"bytes"
	"context"
	"net"
	"testing"

	tracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestNativeOTLPGRPCInteroperabilityAndTenantAuthentication(t *testing.T) {
	publisher := &MemoryPublisher{}
	gateway := Gateway{Authenticator: NewAPIKeyAuthenticator(map[string]string{"tenant-grpc": "grpc-key"}), Publisher: publisher, PolicyVersion: "v2"}
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	RegisterGRPC(server, gateway)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	connection, err := grpc.NewClient("passthrough:///bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	client := tracev1.NewTraceServiceClient(connection)
	request := &tracev1.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{{TraceId: bytes.Repeat([]byte{3}, 16), SpanId: bytes.Repeat([]byte{4}, 8), Name: "grpc-span"}}}}}}}
	if _, err := client.Export(context.Background(), request); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("missing key status = %v", err)
	}
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-paop-api-key", "grpc-key"))
	if _, err := client.Export(ctx, request); err != nil {
		t.Fatal(err)
	}
	if len(publisher.Envelopes) != 1 || publisher.Envelopes[0].TenantID != "tenant-grpc" || publisher.Envelopes[0].Event.Attributes["telemetry.source"] != "otlp_grpc" {
		t.Fatalf("unexpected gRPC envelope: %#v", publisher.Envelopes)
	}
}
