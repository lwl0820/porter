FROM --platform=$BUILDPLATFORM node:24.22.0-bookworm-slim@sha256:a5f4366e834695d5f363b920c104953a535d3a9864d1bdf74566a0ebe01daee5 AS frontend
WORKDIR /src/web
ENV npm_config_cache=/src/.tmp/npm-cache TMPDIR=/src/.tmp
COPY web/package.json web/package-lock.json ./
RUN mkdir -p /src/.tmp && npm ci
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.27.2-trixie@sha256:47667934874913a449b96faa7ef4777fa285d9ec29126ee80ed61c45b7cccf0e AS build
WORKDIR /src
ENV TMPDIR=/src/.tmp GOTMPDIR=/src/.tmp GOCACHE=/src/.tmp/go-cache GOPATH=/src/.tmp/go-path GOMODCACHE=/src/.tmp/go-mod
COPY go.mod go.sum ./
RUN mkdir -p /src/.tmp/runtime-data && go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY web/embed.go ./web/embed.go
COPY --from=frontend /src/web/dist/ ./web/dist/
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -tags porter_docker -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /src/.tmp/porterd ./cmd/porterd

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /src/.tmp/porterd /porterd
COPY --from=build /src/.tmp/runtime-data /data
ENV PORTER_HTTP_ADDR=0.0.0.0:8080 PORTER_DB_PATH=/data/porter.db
EXPOSE 8080/tcp
ENTRYPOINT ["/porterd"]
