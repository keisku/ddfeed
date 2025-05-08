# ddfeed

## Overview

This is a simple web application that enables you to test Datadog products such as APM, DBM, Logs, RUM, etc.

![ddfeed architecture](./ddfeed.png)

## Prerequisites

- Apple Silicon Mac
- [Docker Desktop on Mac](https://docs.docker.com/desktop/setup/install/mac-install/)

## Getting Started

Write your own configs or credentials to `.env` file after copying the template file.

```bash
cp .template.env .env
```

Build and run containers.

```bash
# If you want to try Datadog Tracer.
APM_TARGET=dd COMPOSE_BAKE=true docker compose up -d --build

# If you want to try OTel Tracer.
APM_TARGET=otel COMPOSE_BAKE=true docker compose up -d --build
```

For checking the app status, logs or executing commands in the containers, you can use other `docker compose` commands.

Examples:

- `docker compose exec agent agent status`: Check the status of the Datadog Agent.
- `docker compose logs backend`: Check the logs of the Backend service.
- `docker compose down`: Stop all services.

## Services

### Frontend

- Nginx-based web server serving the UI.
- RUM Automatic instrumentation is enabled.

### Gateway

- Acts as an API Gateway.
- Routes requests from UI to Backend service.
- CORS is enabled for the UI.
- Technically, we don't need this service, but it's useful for understanding the proxy tracing.

### Backend

- Go-based REST API service providing endpoints for post and comment management.
- Supports both Datadog and OpenTelemetry tracing. You can switch the tracer by `./run.sh dd` or `./run.sh otel`.

### MySQL

- Stores posts and comments.

### Valkey

- Used for caching post contents and comment counts.

### Datadog Agent

- Collects traces, logs, and metrics from the containers.
- DBM is enabled for MySQL.
