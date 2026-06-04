# syntax=docker/dockerfile:1

FROM golang:1.23-alpine AS builder

ENV GOTOOLCHAIN=auto

WORKDIR /src

RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/lanroom .

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata wget iproute2 su-exec \
    && adduser -D -h /app -u 1000 lanroom

WORKDIR /app

COPY --from=builder /out/lanroom /app/lanroom
COPY scripts/docker-entrypoint.sh /app/docker-entrypoint.sh

RUN chmod +x /app/docker-entrypoint.sh \
    && mkdir -p /data/uploads \
    && chown -R lanroom:lanroom /app /data/uploads

# entrypoint 以 root 修正卷权限后 su-exec 降权
USER root

ENV LANROOM_UPLOAD_DIR=/data/uploads

EXPOSE 8787

VOLUME ["/data/uploads"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
  CMD wget -q --spider http://127.0.0.1:8787/api/info || exit 1

ENTRYPOINT ["/app/docker-entrypoint.sh"]
CMD ["-port", "8787"]
