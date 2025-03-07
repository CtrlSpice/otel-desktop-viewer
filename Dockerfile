FROM golang:1.24 AS golang

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get -y update

RUN go install github.com/CtrlSpice/otel-desktop-viewer@latest

FROM ubuntu:24.04

COPY --from=golang /go/bin/otel-desktop-viewer /root/otel-desktop-viewer

EXPOSE 8000
EXPOSE 4317
EXPOSE 4318

CMD [ "/root/otel-desktop-viewer", "--host", "0.0.0.0", "--grpc", "4317", "--http", "4318",  "--browser", "8000" ]
# docker --debug build --tag davetron5000/otel-desktop-viewer:go-1.24 .
