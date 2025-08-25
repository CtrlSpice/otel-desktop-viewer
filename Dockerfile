FROM golang:1.24 AS golang

# Install build and runtime dependencies for CGO
RUN apt-get update && apt-get install -y \
    gcc g++ git \
    ca-certificates \
    libstdc++6 \
    && rm -rf /var/lib/apt/lists/*

# Copy source code
WORKDIR /app
COPY . .

# Build the application from source
RUN go build -o otel-desktop-viewer .

FROM debian:latest

# Copy runtime dependencies from build stage
COPY --from=golang /usr/lib/*/libstdc++.so.6* /usr/lib/
COPY --from=golang /etc/ssl/certs /etc/ssl/certs

# Copy the built application
COPY --from=golang /app/otel-desktop-viewer /root/otel-desktop-viewer

EXPOSE 8000
EXPOSE 4317
EXPOSE 4318

CMD [ "/root/otel-desktop-viewer", "--host", "0.0.0.0", "--grpc", "4317", "--http", "4318",  "--browser-port", "8000" ]
