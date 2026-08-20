FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /gateway ./cmd/gateway
RUN CGO_ENABLED=0 go build -o /persist ./cmd/persist
RUN CGO_ENABLED=0 go build -o /query ./cmd/query

FROM build AS migrate
ENTRYPOINT ["go", "run", "./cmd/migrate"]

FROM build AS clickhouse-migrate
ENTRYPOINT ["go", "run", "./cmd/clickhouse-migrate"]

FROM gcr.io/distroless/static-debian12:nonroot AS persist
COPY --from=build /persist /persist
USER nonroot:nonroot
ENTRYPOINT ["/persist"]

FROM gcr.io/distroless/static-debian12:nonroot AS query
COPY --from=build /query /query
USER nonroot:nonroot
EXPOSE 8081
ENTRYPOINT ["/query"]

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /gateway /gateway
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/gateway"]
