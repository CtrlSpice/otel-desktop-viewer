FROM node:24-bookworm AS frontend

WORKDIR /frontend
COPY desktopexporter/internal/frontend/package.json desktopexporter/internal/frontend/package-lock.json ./
RUN npm ci
COPY desktopexporter/internal/frontend/ ./
RUN npm run build

FROM golang:1.26 AS golang

# Install build and runtime dependencies for CGO
RUN apt-get update && apt-get install -y \
    gcc g++ git \
    ca-certificates \
    libstdc++6 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Embed the UI built in the frontend stage (not committed static/)
RUN rm -rf desktopexporter/internal/server/static/*
COPY --from=frontend /frontend/dist/ desktopexporter/internal/server/static/

RUN go build -o otel-desktop-viewer .

# Full image (not slim): the duckdb-go cgo runtime needs the complete
# library surface, which slim variants have broken before.
FROM debian:13

# Copy runtime dependencies from build stage
COPY --from=golang /usr/lib/*/libstdc++.so.6* /usr/lib/
COPY --from=golang /etc/ssl/certs /etc/ssl/certs

# Copy the built application
COPY --from=golang /app/otel-desktop-viewer /root/otel-desktop-viewer

# Add metadata labels
LABEL org.opencontainers.image.title="OpenTelemetry Desktop Viewer"
LABEL org.opencontainers.image.description="A desktop application for viewing and analyzing OpenTelemetry traces, metrics, and logs locally"
LABEL org.opencontainers.image.vendor="CtrlSpice"
LABEL org.opencontainers.image.source="https://github.com/CtrlSpice/otel-desktop-viewer"
LABEL org.opencontainers.image.licenses="Apache-2.0"
LABEL org.opencontainers.image.url="https://github.com/CtrlSpice/otel-desktop-viewer"

EXPOSE 8000
EXPOSE 4317
EXPOSE 4318

CMD [ "/root/otel-desktop-viewer", "--host", "0.0.0.0", "--grpc", "4317", "--http", "4318",  "--browser-port", "8000" ]
