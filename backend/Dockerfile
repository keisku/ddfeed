FROM golang:1.24 AS dd-build
WORKDIR /build
RUN go install github.com/DataDog/orchestrion@latest
COPY orchestrion.tool.go orchestrion.tool.go
COPY cmd cmd
COPY internal internal
COPY go.mod go.mod
COPY go.sum go.sum
RUN go generate ./...
RUN CGO_ENABLED=0 orchestrion go build -ldflags "-s -w" -o app ./cmd/dd/main.go
FROM debian:bookworm-slim AS dd
COPY --from=dd-build /build/app /run/app
ENTRYPOINT ["/run/app"]

FROM golang:1.24 AS otel-build
WORKDIR /build
COPY cmd cmd
COPY internal internal
COPY go.mod go.mod
COPY go.sum go.sum
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o app ./cmd/otel/main.go
FROM debian:bookworm-slim AS otel
COPY --from=otel-build /build/app /run/app
ENTRYPOINT ["/run/app"]
