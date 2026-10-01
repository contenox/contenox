# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -o /out/vfsapitest ./internal/tools/vfsapitest

FROM alpine:3.23 AS vfs-test-host
COPY --from=build /out/vfsapitest /usr/local/bin/vfsapitest
HEALTHCHECK --interval=2s --timeout=3s --retries=15 \
    CMD wget -q -O /dev/null http://127.0.0.1:8080/ready || exit 1
ENTRYPOINT ["/usr/local/bin/vfsapitest"]
CMD ["-addr", "0.0.0.0:8080"]

FROM alpine:3.23 AS api-tests
RUN apk add --no-cache ca-certificates python3 py3-pip
COPY internal/apitests/requirements.txt /tests/requirements.txt
RUN python3 -m venv /opt/pytest \
    && /opt/pytest/bin/pip install --no-cache-dir -r /tests/requirements.txt
COPY internal/apitests /tests/internal/apitests
WORKDIR /tests
ENTRYPOINT ["/opt/pytest/bin/python", "-m", "pytest"]
CMD ["internal/apitests", "-q", "-p", "no:cacheprovider"]
