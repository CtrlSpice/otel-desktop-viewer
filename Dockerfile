FROM golang:1.24 AS golang

# Install build dependencies for CGO
RUN apt-get update && apt-get install -y gcc g++ git

# Install the application
RUN go install github.com/CtrlSpice/otel-desktop-viewer@latest

FROM debian:latest

# Install runtime dependencies for DuckDB and C++ libraries
RUN apt-get update && apt-get install -y \
    ca-certificates \
    libstdc++6 \
    && rm -rf /var/lib/apt/lists/*

COPY --from=golang /go/bin/otel-desktop-viewer /root/otel-desktop-viewer

EXPOSE 8000
EXPOSE 4317
EXPOSE 4318

CMD [ "/root/otel-desktop-viewer", "--host", "0.0.0.0", "--grpc", "4317", "--http", "4318",  "--browser-port", "8000" ]
