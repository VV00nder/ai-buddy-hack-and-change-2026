# syntax=docker/dockerfile:1
# Сборка AI-buddy: фронтенд (Node) + два Go-сервиса. Цели: buddy, corpmock, test.

FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json ./
RUN npm install --no-audit --no-fund --loglevel=error
COPY web/ ./
RUN npm run build

FROM golang:1.24-alpine AS gobuild
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/buddy ./cmd/buddy \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/corpmock ./cmd/corpmock

FROM alpine:3.20 AS buddy
RUN adduser -D -H app && apk add --no-cache wget
COPY --from=gobuild /out/buddy /app/buddy
COPY --from=web /web/dist /app/web
ENV WEB_DIR=/app/web BUDDY_ADDR=:8080
USER app
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --retries=5 CMD wget -qO- http://127.0.0.1:8080/api/health || exit 1
ENTRYPOINT ["/app/buddy"]

FROM alpine:3.20 AS corpmock
RUN adduser -D -H app && apk add --no-cache wget
COPY --from=gobuild /out/corpmock /app/corpmock
ENV CORP_ADDR=:8090
USER app
EXPOSE 8090
HEALTHCHECK --interval=10s --timeout=3s --retries=5 CMD wget -qO- http://127.0.0.1:8090/healthz || exit 1
ENTRYPOINT ["/app/corpmock"]

# Тесты с отчётом о покрытии (docker compose --profile test run --rm tests)
FROM golang:1.24-alpine AS test
RUN apk add --no-cache nodejs npm bash
WORKDIR /repo
COPY backend/go.mod backend/go.sum backend/
RUN cd backend && go mod download
COPY web/package.json web/
RUN cd web && npm install --no-audit --no-fund --loglevel=error
COPY . .
CMD ["bash", "scripts/test.sh"]
