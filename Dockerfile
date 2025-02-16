FROM golang:1.24

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get -y update

RUN go install github.com/CtrlSpice/otel-desktop-viewer@latest

EXPOSE 8000
EXPOSE 4317
EXPOSE 4318

CMD [ "otel-desktop-viewer", "--host", "0.0.0.0", "--grpc", "4317", "--http", "4318",  "--browser", "8000" ]
# docker --debug build --tag davetron5000/otel-desktop-viewer:go-1.24 .
