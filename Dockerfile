# Base images pinned by digest; bump them on purpose, not by accident.
# Built by .github/workflows/build.yml for the k3s cluster. node-1 uses Dockerfile.prod until the cutover.
FROM golang:1.26.10-alpine3.24@sha256:27f2d3b83cfc5803c8078f8e730af0f7c431bf067afaec8f60a36fb763187cba AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /server .

# Static binary, CA certificates, time zones, no shell, runs as uid 65532 (Pod Security "restricted").
# migrations/ isn't copied: the server never reads it (they're applied by hand, Makefile).
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

COPY --from=build /server /server

EXPOSE 8080

USER 65532:65532

ENTRYPOINT ["/server"]
