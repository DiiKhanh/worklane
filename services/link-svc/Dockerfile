# Build with the repository root as context.
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/link-svc ./services/link-svc

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/link-svc /link-svc
COPY --from=build /src/db/link/migrations /db/link/migrations
ENV MIGRATIONS_DIR=/db/link/migrations
EXPOSE 8890
ENTRYPOINT ["/link-svc"]
