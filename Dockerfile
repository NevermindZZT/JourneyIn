FROM node:24-alpine AS web-build
WORKDIR /src/web
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN corepack enable && pnpm install --frozen-lockfile
COPY web ./
RUN pnpm build

FROM golang:1.26-alpine AS go-build
ARG VERSION=0.2.0
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN CGO_ENABLED=0 go mod download
COPY . ./
COPY --from=web-build /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/journeyin ./cmd/journeyin

FROM gcr.io/distroless/static-debian12:nonroot
ARG VERSION=0.2.0
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.title=JourneyIn \
      org.opencontainers.image.description="Map-first travel planning" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.source="https://github.com/NevermindZZT/JourneyIn"
COPY --from=go-build /out/journeyin /journeyin
VOLUME ["/data"]
EXPOSE 8080
ENTRYPOINT ["/journeyin"]
