FROM --platform=$BUILDPLATFORM node:22-alpine AS frontend
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run generate

FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS backend
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY --from=frontend /src/web/.output/public/ internal/web/public/
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /goinmetrics ./cmd/goinmetrics

FROM alpine:3.22
# Numeric identity works on both architectures without executing target binaries
# during the build. Runtime needs no shell initialization or generated users.
COPY --from=backend /goinmetrics /usr/local/bin/goinmetrics
USER 65532:65532
LABEL org.opencontainers.image.title="GoInMetrics" \
      org.opencontainers.image.description="Local Linux server monitoring" \
      org.opencontainers.image.source="https://github.com/D1n8/linux-monitoring"
ENTRYPOINT ["/usr/local/bin/goinmetrics"]
