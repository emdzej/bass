# syntax=docker/dockerfile:1.7

FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown
RUN CGO_ENABLED=0 GOOS=linux \
    go build \
      -trimpath \
      -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
      -o /out/bass \
      ./cmd/bass \
 && mkdir -p /out/data \
 && chown -R 65532:65532 /out/data

FROM gcr.io/distroless/static-debian12:nonroot
USER nonroot:nonroot
COPY --from=build /out/bass /bass
# Pre-seed /data with nonroot ownership so a freshly-created named volume
# (root-owned by default) inherits writable permissions on first mount.
# Distroless has no shell, so we cannot chown at runtime — this is the
# canonical workaround.
COPY --from=build --chown=65532:65532 /out/data /data
VOLUME ["/data"]
ENV BASS_DB_PATH=/data/bass.db \
    BASS_ADDR=:8080
EXPOSE 8080
ENTRYPOINT ["/bass"]
CMD ["serve"]
