# syntax=docker/dockerfile:experimental

FROM golang:1.20-alpine as dev
RUN apk add --no-cache git ca-certificates
RUN adduser -D appuser
COPY . /src/
WORKDIR /src

ENV GO111MODULE=on
RUN --mount=type=cache,sharing=locked,id=gomod,target=/go/pkg/mod/cache \
    --mount=type=cache,sharing=locked,id=goroot,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -ldflags '-s -w -extldflags -static' -o kube-vip-gateway-api

FROM scratch
COPY --from=dev /src/kube-vip-gateway-api /
CMD ["/kube-vip-gateway-api"]