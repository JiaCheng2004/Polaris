FROM golang:1.26.2-alpine AS builder

WORKDIR /src
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/polaris ./cmd/polaris
RUN mkdir -p /image-root/etc/polaris /image-root/var/lib/polaris && touch /image-root/var/lib/polaris/.keep

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=builder /out/polaris /usr/local/bin/polaris
COPY --from=builder --chown=nonroot:nonroot /image-root/etc/polaris /etc/polaris
COPY --from=builder --chown=nonroot:nonroot /image-root/var/lib/polaris /var/lib/polaris

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/polaris"]
CMD ["--config", "/etc/polaris/polaris.yaml"]
