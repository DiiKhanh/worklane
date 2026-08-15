# Multi-stage build. Context must be the repo root (single Go module with shared pkg).
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/otp-dispatcher ./services/otp-dispatcher

FROM gcr.io/distroless/static-debian12
WORKDIR /
COPY --from=build /out/otp-dispatcher /otp-dispatcher
ENTRYPOINT ["/otp-dispatcher"]
