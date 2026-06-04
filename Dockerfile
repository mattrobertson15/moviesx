FROM golang:1.25-bookworm AS builder

WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w" -o /moviesx ./cmd/moviesx

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /moviesx /moviesx
COPY --from=builder /src/src/data /data
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/moviesx"]
