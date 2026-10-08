# ── Stage 1: Build Frontend SPA ──────────────────────────────────────────────
FROM node:22-alpine AS frontend-builder
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci --silent
COPY web/ ./
RUN npm run build

# ── Stage 2: Build Backend Binary ───────────────────────────────────────────
FROM golang:1.26-alpine AS backend-builder
WORKDIR /app
RUN apk add --no-cache git gcc musl-dev
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend-builder /app/web/dist ./web/dist
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o kestrel-server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o kestrel-mcp ./cmd/mcp-stdio

# ── Stage 3: Production Runtime ─────────────────────────────────────────────
FROM alpine:3.19
LABEL maintainer="Kestrel Team"
WORKDIR /opt/kestrel

# Install standard security tools and runtime utilities
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    bash \
    curl \
    python3 \
    py3-pip \
    nmap \
    bind-tools \
    git \
    openssl \
    && rm -rf /var/cache/apk/*

# Copy binaries and assets
COPY --from=backend-builder /app/kestrel-server /opt/kestrel/kestrel-server
COPY --from=backend-builder /app/kestrel-mcp /opt/kestrel/kestrel-mcp
COPY --from=backend-builder /app/web/dist /opt/kestrel/web/dist
COPY config.example.yaml /opt/kestrel/config.example.yaml
COPY agents /opt/kestrel/agents
COPY roles /opt/kestrel/roles
COPY tools /opt/kestrel/tools
COPY skills /opt/kestrel/skills
COPY knowledge_base /opt/kestrel/knowledge_base
COPY requirements.txt /opt/kestrel/requirements.txt

# Install python dependencies in container
RUN python3 -m venv /opt/kestrel/venv && \
    /opt/kestrel/venv/bin/pip install --no-cache-dir -r requirements.txt || true

# Storage directories
RUN mkdir -p /opt/kestrel/data /opt/kestrel/log /opt/kestrel/scratch

EXPOSE 8080 8443
VOLUME ["/opt/kestrel/data", "/opt/kestrel/log"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD curl -f http://127.0.0.1:8080/api/system/info || exit 1

ENTRYPOINT ["/opt/kestrel/kestrel-server"]
CMD ["-config", "/opt/kestrel/config.yaml"]
