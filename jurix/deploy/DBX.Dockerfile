# syntax=docker/dockerfile:1
# Private JURIX variant of deploy/Dockerfile; includes both strict-mode binaries.
FROM node:22-alpine AS dashboard
WORKDIR /build/dashboard
COPY dashboard/package.json dashboard/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY dashboard/ ./
RUN npm run build

FROM golang:1.25-alpine AS build
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY dashboard/*.go ./dashboard/
COPY --from=dashboard /build/dashboard/dist ./dashboard/dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/dbx-orchestrator ./cmd/dbx-orchestrator \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/dbx-server ./cmd/dbx-server

FROM alpine:3.23
RUN apk add --no-cache ca-certificates tzdata wget \
    && addgroup -g 10002 dbx \
    && adduser -D -H -u 10002 -G dbx dbx \
    && mkdir -p /data \
    && chown 10002:10002 /data \
    && chmod 0700 /data
WORKDIR /data
ENV DBX_DATA_DIR=/data DBX_ISOLATION_MODE=strict DBX_PRODUCTION=1
COPY --from=build /out/dbx-orchestrator /out/dbx-server /usr/local/bin/
COPY jurix/deploy/dbx-entrypoint.sh /usr/local/bin/dbx-entrypoint.sh
USER 10002:10002
EXPOSE 8000 6380
ENTRYPOINT ["/bin/sh", "/usr/local/bin/dbx-entrypoint.sh"]
CMD ["-tls-cert=/run/secrets/dbx_tls_cert", "-tls-key=/run/secrets/dbx_tls_key", "-state-file=/data/tenants.json", "-admin-file=/data/admin.json"]
