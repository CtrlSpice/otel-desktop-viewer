FROM golang:1.24-alpine AS golang

RUN go install github.com/CtrlSpice/otel-desktop-viewer@latest

FROM alpine:3

COPY --from=golang /go/bin/otel-desktop-viewer /root/otel-desktop-viewer

EXPOSE 8000
EXPOSE 4317
EXPOSE 4318

CMD [ "/root/otel-desktop-viewer", "--host", "0.0.0.0", "--grpc", "4317", "--http", "4318",  "--browser", "8000" ]
# docker --debug build --tag davetron5000/otel-desktop-viewer:go-1.24 .
