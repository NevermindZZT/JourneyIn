FROM node:24-alpine AS web-build
WORKDIR /src/web
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN corepack enable && pnpm install --frozen-lockfile
COPY web ./
RUN pnpm build

FROM golang:1.26-alpine AS go-build
ARG VERSION=0.3.0
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN CGO_ENABLED=0 go mod download
COPY . ./
COPY --from=web-build /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/journeyin ./cmd/journeyin

FROM alpine:3.22 AS runtime-data
RUN install -d -o 65532 -g 65532 /data

FROM gcr.io/distroless/static-debian12:nonroot
ARG VERSION=0.3.0
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.title=JourneyIn \
      org.opencontainers.image.description="Map-first travel planning" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.source="https://github.com/NevermindZZT/JourneyIn"
COPY --from=go-build /out/journeyin /journeyin
COPY --from=runtime-data --chown=65532:65532 /data /data

# The single Go binary starts as root only to prepare the fixed /data mount.
# It drops permanently to 65532:65532 before opening SQLite or serving HTTP.
ENV JOURNEYIN_DATA_DIR=/data/journeyin.db \
    JOURNEYIN_DOCKER_RUNTIME=1 \
    JOURNEYIN_DOCKER_AUTO_FIX_PERMISSIONS=1
USER 0:0
VOLUME ["/data"]
EXPOSE 8080
ENTRYPOINT ["/journeyin"]
