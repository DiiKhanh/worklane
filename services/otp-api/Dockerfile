# Multi-stage build. Context must be the repo root (single Go module with shared pkg).
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/otp-api ./services/otp-api

# Distroless: tiny, no shell, no package manager - smaller attack surface.
FROM gcr.io/distroless/static-debian12
WORKDIR /
COPY --from=build /out/otp-api /otp-api
# Migrations are read at startup (golang-migrate needs the .sql files at runtime).
COPY --from=build /src/db/otp/migrations /db/otp/migrations
ENV MIGRATIONS_DIR=/db/otp/migrations
EXPOSE 8888
ENTRYPOINT ["/otp-api"]
