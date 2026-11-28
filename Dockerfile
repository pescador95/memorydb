# syntax=docker/dockerfile:1

ARG GO_VERSION=1.27.0

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS build

WORKDIR /src

COPY . .

ARG TARGETOS
ARG TARGETARCH

RUN mkdir -p /out && \
    CGO_ENABLED=0 \
    GOOS=${TARGETOS:-linux} \
    GOARCH=${TARGETARCH:-amd64} \
    go -C /src/server build \
        -trimpath \
        -ldflags="-s -w" \
        -o /out/memorydb \
        ./cmd/memorydb

FROM scratch AS runtime

COPY --from=build /out/memorydb /memorydb

ENV MEMORYDB_ADDR=:6380
ENV MEMORYDB_HEALTH_ADDR=:8081

EXPOSE 6380/tcp
EXPOSE 8081/tcp

USER 65532:65532

STOPSIGNAL SIGTERM

ENTRYPOINT ["/memorydb"]