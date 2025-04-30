#!/bin/bash

if [ "$1" != "dd" ] && [ "$1" != "otel" ]; then
	echo "Error: dd or otel is required."
	exit 1
fi

# Start MySQL, Valkey, and Datadog Agent first.
# They are required by the backend and frontend.
docker compose up -d mysql valkey agent

# Build and start the backend
dockerfile_backend=$(mktemp)
cat > "$dockerfile_backend" <<EOF
FROM golang:1.24 AS builder
WORKDIR /build
EOF
if [ "$1" = "dd" ]; then
cat >> "$dockerfile_backend" <<EOF
RUN go install github.com/DataDog/orchestrion@latest
COPY orchestrion.tool.go orchestrion.tool.go
EOF
fi
cat >> "$dockerfile_backend" <<EOF
COPY cmd cmd
COPY internal internal
COPY go.mod go.mod
COPY go.sum go.sum
EOF
if [ "$1" = "dd" ]; then
cat >> "$dockerfile_backend" <<EOF
RUN go generate ./...
RUN CGO_ENABLED=0 orchestrion go build -ldflags "-s -w" -o app ./cmd/dd/main.go
EOF
elif [ "$1" = "otel" ]; then
cat >> "$dockerfile_backend" <<EOF
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o app ./cmd/otel/main.go
EOF
fi
cat >> "$dockerfile_backend" <<EOF
FROM debian:bookworm-slim
COPY --from=builder /build/app /run/app
ENTRYPOINT ["/run/app"]
EOF
docker build -t ddfeed-backend -f "$dockerfile_backend" ./backend
docker compose up -d backend

# Build and start the frontend
dockerfile_frontend=$(mktemp)
if [ "$1" = "dd" ]; then
cat > "$dockerfile_frontend" <<EOF
FROM nginx:1.27.3
EOF
elif [ "$1" = "otel" ]; then
cat > "$dockerfile_frontend" <<EOF
FROM nginx:1.27-otel
EOF
fi
cat >> "$dockerfile_frontend" <<EOF
RUN apt-get update -y
EOF
if [ "$1" = "dd" ]; then
cat >> "$dockerfile_frontend" <<EOF
ADD https://rum-auto-instrumentation.s3.amazonaws.com/installer/latest/install-proxy-datadog.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/install-proxy-datadog.sh
COPY --chmod=755 ./dd.sh /usr/local/bin/dd.sh
CMD ["/usr/local/bin/dd.sh"]
EOF
elif [ "$1" = "otel" ]; then
# TODO: Not sure why nginx-otel spans are not connected with other spans.
cat >> "$dockerfile_frontend" <<EOF
COPY --chmod=644 ./nginx.otel.conf /etc/nginx/nginx.conf
COPY --chmod=644 ./default.otel.conf /etc/nginx/conf.d/default.conf
EOF
fi
cat >> "$dockerfile_frontend" <<EOF
COPY --chmod=755 ./html /usr/share/nginx/html
EOF
docker build -t ddfeed-frontend -f "$dockerfile_frontend" ./frontend
docker compose up -d frontend

# Build and start the Gateway
docker compose build gateway --build-arg APM_TYPE="$1"
docker compose up -d gateway

# Print the Dockerfile for debugging.
echo "============== backend Dockerfile =============="
cat "$dockerfile_backend"
rm "$dockerfile_backend"
echo "============== frontend Dockerfile =============="
cat "$dockerfile_frontend"
rm "$dockerfile_frontend"

# Open the frontend in the browser.
open http://localhost:16163
