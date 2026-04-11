FROM golang:1.26.2-alpine AS builder

WORKDIR /src
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/polaris ./cmd/polaris

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=builder /out/polaris /usr/local/bin/polaris

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/polaris"]
CMD ["--config", "/etc/polaris/polaris.yaml"]
