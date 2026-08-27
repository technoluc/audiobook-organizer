# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM node:22-alpine AS web-builder
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

ARG TARGETPLATFORM
ARG BUILDPLATFORM
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

WORKDIR /app
COPY . .
# Vite writes the production UI into ../internal/server/static. Replace the
# checked-in bundle with the UI built from the current source tree so Docker
# builds include web/src changes.
COPY --from=web-builder /src/internal/server/static ./internal/server/static

# Build for the target platform with the freshly generated web UI embedded.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=${TARGETVARIANT#v} \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o audiobook-organizer

FROM --platform=$TARGETPLATFORM alpine:latest

WORKDIR /app
COPY --from=builder /app/audiobook-organizer .

EXPOSE 8080

ENTRYPOINT ["/app/audiobook-organizer"]
