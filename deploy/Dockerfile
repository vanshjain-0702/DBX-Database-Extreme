# Stage 1: Build the dashboard
FROM node:20-alpine AS dashboard-builder
WORKDIR /app/dashboard
COPY dashboard/package*.json ./
RUN npm ci
COPY dashboard/ ./
RUN npm run build

# Stage 2: Build the Go orchestrator
FROM golang:1.25-alpine AS go-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Copy the pre-built dashboard into the Go build context
COPY --from=dashboard-builder /app/dashboard/dist ./dashboard/dist
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /dbx-orchestrator ./cmd/dbx-orchestrator

# Stage 3: Minimal production image
FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata wget
WORKDIR /app
COPY --from=go-builder /dbx-orchestrator /usr/local/bin/dbx-orchestrator
RUN mkdir -p /data /certs
EXPOSE 8000
VOLUME ["/data"]
ENTRYPOINT ["dbx-orchestrator", "-insecure-http=true", "-state-file=/data/tenants.json", "-admin-file=/data/admin.json", "-raftdir=/data/raft"]
