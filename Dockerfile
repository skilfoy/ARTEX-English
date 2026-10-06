# syntax=docker/dockerfile:1
FROM node:24-bookworm-slim AS frontend
WORKDIR /src/web
ENV NODE_OPTIONS=--max-old-space-size=3072
COPY web/package.json web/package-lock.json ./
RUN npm ci --ignore-scripts
COPY web/ ./
RUN npm run build:static

FROM golang:1.26.3-bookworm AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/web/out /src/server/webui/dist
RUN CGO_ENABLED=0 go build -tags embedui -o /out/artex ./cmd/artex

FROM python:3.12-slim-bookworm
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl git jq ripgrep nmap dnsutils iputils-ping wget vim unzip \
    netcat-openbsd inetutils-telnet whois \
    && curl -fsSL https://deb.nodesource.com/setup_20.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && npm install -g @playwright/mcp@0.0.83 @playwright/cli@0.1.22 playwright@1.63.0 \
    && playwright-cli --help \
    && playwright install --with-deps chromium \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=backend /out/artex /app/artex
COPY start.sh /app/start.sh
COPY skills/ /app/skills/
RUN chmod +x /app/artex /app/start.sh
VOLUME ["/app/data", "/app/state"]
EXPOSE 8787 8788
ENTRYPOINT ["/app/start.sh"]
CMD ["-addr", ":8787", "-proxy", ":8788"]
