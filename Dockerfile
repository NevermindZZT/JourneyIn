FROM node:24-alpine AS web-build
WORKDIR /src/web
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN corepack enable && pnpm install --frozen-lockfile
COPY web ./
RUN pnpm build

FROM golang:1.26-alpine AS go-build
WORKDIR /src
COPY go.mod go.sum ./
RUN CGO_ENABLED=0 go mod download
COPY . ./
COPY --from=web-build /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=docker" -o /out/journeyin ./cmd/journeyin

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=go-build /out/journeyin /journeyin
VOLUME ["/data"]
EXPOSE 8080
ENTRYPOINT ["/journeyin"]
