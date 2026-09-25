# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -o /out/contenox ./cmd/contenox

FROM alpine:3.23
RUN apk add --no-cache ca-certificates curl su-exec \
    && addgroup -g 10001 contenox \
    && adduser -D -u 10001 -G contenox -h /tmp contenox
COPY --from=build /out/contenox /usr/local/bin/contenox
COPY scripts/gateway-container-entrypoint.sh /usr/local/bin/gateway-container-entrypoint
COPY LICENSE /usr/share/licenses/contenox/LICENSE
USER contenox
WORKDIR /tmp
EXPOSE 11435
HEALTHCHECK --interval=15s --timeout=3s --start-period=20s \
    CMD curl --fail --silent http://127.0.0.1:11435/api/version >/dev/null || exit 1
ENTRYPOINT ["contenox"]
CMD ["gateway", "serve", "--listen", "0.0.0.0:11435"]
