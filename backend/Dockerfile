FROM golang:1.26 AS build
WORKDIR /build
COPY cmd cmd
COPY internal internal
COPY go.mod go.sum ./

# Build for Datadog
FROM build AS dd-build
RUN go install github.com/DataDog/orchestrion@latest
COPY orchestrion.tool.go .
RUN go generate ./...
# Below line is to avoid "Failed to pin orchestrion" error (https://github.com/DataDog/orchestrion/issues/491#issuecomment-2577822513)
RUN orchestrion pin 
RUN CGO_ENABLED=0 orchestrion go build -ldflags "-s -w" -o app ./cmd/dd/main.go

# Build for OpenTelemetry
FROM build AS otel-build
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o app ./cmd/otel/main.go

# Final stages for Datadog
FROM debian:bookworm-slim AS dd
RUN <<EOF
apt-get update
apt-get install -y curl
rm -rf /var/lib/apt/lists/*
EOF
COPY --from=dd-build /build/app /run/app
ENTRYPOINT ["/run/app"]

# Final stages for OpenTelemetry
FROM debian:bookworm-slim AS otel
RUN <<EOF
apt-get update
apt-get install -y curl
rm -rf /var/lib/apt/lists/*
EOF
COPY --from=otel-build /build/app /run/app
ENTRYPOINT ["/run/app"]
