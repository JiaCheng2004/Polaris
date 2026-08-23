# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM golang:1.27.0-alpine AS builder

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath \
      -ldflags="-s -w \
        -X main.version=${VERSION} \
        -X main.commit=${COMMIT} \
        -X main.buildDate=${BUILD_DATE}" \
      -o /out/polaris ./cmd/polaris

RUN mkdir -p /image-root/etc/polaris /image-root/var/lib/polaris/files && \
    cp -R config/providers /image-root/etc/polaris/providers && \
    cp -R config/routing /image-root/etc/polaris/routing && \
    cp deployments/polaris.edge.yaml /image-root/etc/polaris/polaris.yaml && \
    touch /image-root/var/lib/polaris/.keep /image-root/var/lib/polaris/files/.keep

FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.source="https://github.com/JiaCheng2004/Polaris"

WORKDIR /app
COPY --from=builder /out/polaris /usr/local/bin/polaris
COPY --from=builder --chown=nonroot:nonroot /image-root/etc/polaris /etc/polaris
COPY --from=builder --chown=nonroot:nonroot /image-root/var/lib/polaris /var/lib/polaris

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/polaris"]
CMD ["--config", "/etc/polaris/polaris.yaml"]
