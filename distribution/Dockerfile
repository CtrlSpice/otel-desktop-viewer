FROM alpine:3.16 as certs
RUN apk --update add ca-certificates

FROM scratch

ARG TARGETOS
ARG TARGETARCH
ARG USER_UID=10001
USER ${USER_UID}

COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --chmod=755 ${TARGETOS}/${TARGETARCH}/otelcol /otelcol
COPY config.yaml /etc/otelcol/config.yaml
ENTRYPOINT ["/otelcol"]
CMD ["--config", "/etc/otelcol/config.yaml"]
EXPOSE 4317 4318 8000