FROM node:22-alpine AS frontend
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run generate

FROM golang:1.25-alpine AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY --from=frontend /src/web/.output/public/ internal/web/public/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /goinmetrics ./cmd/goinmetrics

FROM alpine:3.22
RUN addgroup -g 65532 monitor && adduser -D -H -u 65532 -G monitor monitor
COPY --from=backend /goinmetrics /usr/local/bin/goinmetrics
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/goinmetrics"]
