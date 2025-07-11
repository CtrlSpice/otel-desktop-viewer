FROM ubuntu:25.04 AS golangbuilder

RUN apt-get update && \
    apt-get install -y golang-go ca-certificates git build-essential && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app/otel-desktop-viewer

RUN mkdir -p /go/bin
ENV GOPATH=/go

# Assuming changes that you want to have dockerized have been pushed:
# 1) make a separate change to the Dockerfile here to build the app distro from a specific commit # and test with the Dockerfile_test.go test
# 2) then commit and tag the the Dockerfile change (e.g. 'v0.2.3') and push both, the commit and the tag back
# for everyone to be able to build the image directly from the git repo like:
# 'docker build -t otel-desktop-viewer:ubuntu-25.04 git@github.com:CtrlSpice/otel-desktop-viewer.git#v0.2.3'
RUN go install -v github.com/CtrlSpice/otel-desktop-viewer@latest

FROM ubuntu:25.04

COPY --from=golangbuilder /go/bin/otel-desktop-viewer /root/otel-desktop-viewer

EXPOSE 8000
EXPOSE 4317
EXPOSE 4318

CMD [ "/root/otel-desktop-viewer", "--host", "0.0.0.0", "--grpc", "4317", "--http", "4318",  "--browser-port", "8000" ]