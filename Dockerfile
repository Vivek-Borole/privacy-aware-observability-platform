FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /gateway ./cmd/gateway
RUN CGO_ENABLED=0 go build -o /tailer ./cmd/tailer
RUN CGO_ENABLED=0 go build -o /persist ./cmd/persist
RUN CGO_ENABLED=0 go build -o /query ./cmd/query
RUN CGO_ENABLED=0 go build -o /synthetic-emitter ./cmd/synthetic-emitter
RUN CGO_ENABLED=0 go build -o /synthetic-downstream ./cmd/synthetic-downstream
RUN CGO_ENABLED=0 go build -o /synthetic-worker ./cmd/synthetic-worker
RUN CGO_ENABLED=0 go build -o /retention ./cmd/retention

FROM gcr.io/distroless/static-debian12:nonroot AS retention
COPY --from=build /retention /retention
USER nonroot:nonroot
ENTRYPOINT ["/retention"]

FROM build AS migrate
ENTRYPOINT ["go", "run", "./cmd/migrate"]

FROM build AS bootstrap
ENTRYPOINT ["go", "run", "./cmd/bootstrap"]

FROM build AS clickhouse-migrate
ENTRYPOINT ["go", "run", "./cmd/clickhouse-migrate"]

FROM gcr.io/distroless/static-debian12:nonroot AS persist
COPY --from=build /persist /persist
USER nonroot:nonroot
ENTRYPOINT ["/persist"]

FROM gcr.io/distroless/static-debian12:nonroot AS tailer
COPY --from=build /tailer /tailer
USER nonroot:nonroot
ENTRYPOINT ["/tailer"]

FROM gcr.io/distroless/static-debian12:nonroot AS query
COPY --from=build /query /query
USER nonroot:nonroot
EXPOSE 8081
ENTRYPOINT ["/query"]

FROM gcr.io/distroless/static-debian12:nonroot AS synthetic-downstream
COPY --from=build /synthetic-downstream /synthetic-downstream
USER nonroot:nonroot
EXPOSE 8091
ENTRYPOINT ["/synthetic-downstream"]

FROM gcr.io/distroless/static-debian12:nonroot AS synthetic-worker
COPY --from=build /synthetic-worker /synthetic-worker
USER nonroot:nonroot
EXPOSE 8092
ENTRYPOINT ["/synthetic-worker"]

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /gateway /gateway
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/gateway"]
