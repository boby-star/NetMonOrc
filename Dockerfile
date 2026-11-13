FROM golang:1.25.4-alpine3.22@sha256:d3f0cf7723f3429e3f9ed846243970b20a2de7bae6a5b66fc5914e228d831bbb AS build
WORKDIR /src
ENV GOTOOLCHAIN=local CGO_ENABLED=0
COPY go.mod go.sum ./
RUN --mount=type=secret,id=build_ca \
    if [ -s /run/secrets/build_ca ]; then SSL_CERT_FILE=/run/secrets/build_ca go mod download; else go mod download; fi
COPY cmd ./cmd
COPY internal ./internal
COPY proto ./proto
RUN --mount=type=cache,target=/root/.cache/go-build go build -trimpath -ldflags='-s -w' -o /out/controller ./cmd/netmonorc-controller && \
    go build -trimpath -ldflags='-s -w' -o /out/agent ./cmd/netmonorc-agent && \
    go build -trimpath -ldflags='-s -w' -o /out/netmonorc ./cmd/netmonorc && \
    go build -trimpath -ldflags='-s -w' -o /out/experiment ./cmd/netmonorc-experiment && \
    mkdir -p /runtime/data && chmod 0700 /runtime/data && chown 10001:10001 /runtime/data
FROM prom/blackbox-exporter:v0.27.0@sha256:a50c4c0eda297baa1678cd4dc4712a67fdea713b832d43ce7fcc5f9bea05094d AS exporter
FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build --chmod=0755 /out/controller /usr/local/bin/netmonorc-controller
COPY --from=build --chmod=0755 /out/agent /usr/local/bin/netmonorc-agent
COPY --from=build --chmod=0755 /out/netmonorc /usr/local/bin/netmonorc
COPY --from=build --chmod=0755 /out/experiment /usr/local/bin/netmonorc-experiment
COPY --from=exporter --chmod=0755 /bin/blackbox_exporter /usr/local/bin/blackbox_exporter
COPY --from=build --chown=10001:10001 /runtime /
USER 10001:10001
WORKDIR /data
ENTRYPOINT ["/usr/local/bin/netmonorc-controller"]
