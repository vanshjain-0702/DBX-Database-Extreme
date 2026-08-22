# Stage 1: Build the dashboard before compiling Go. The orchestrator embeds
# dashboard/dist, so a clean checkout must produce it first.
FROM node:20-alpine AS node-builder
WORKDIR /app/dashboard
COPY dashboard/package*.json ./
RUN npm ci
COPY dashboard/ ./
RUN npm run build

# Stage 2: Build Go binaries with the generated dashboard embedded.
FROM golang:1.25-alpine AS go-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=node-builder /app/dashboard/dist ./dashboard/dist
RUN CGO_ENABLED=0 GOOS=linux go build -o bin/server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -o bin/orchestrator ./cmd/orchestrator

# Stage 3: Final Minimal Image
FROM alpine:latest
WORKDIR /dbx

# Copy binaries
COPY --from=go-builder /app/bin/server ./bin/
COPY --from=go-builder /app/bin/orchestrator ./bin/

# Copy default configuration. TLS certificates are mounted at runtime and are
# deliberately never baked into the image.
COPY configs/ ./configs/

# Copy built dashboard
COPY --from=node-builder /app/dashboard/dist ./dashboard/dist

# Set permissions for execution
RUN chmod +x ./bin/server ./bin/orchestrator

# Expose Orchestrator and API
EXPOSE 8000

# The orchestrator spawns sub-processes for DBX tenants internally.
CMD ["./bin/orchestrator"]
