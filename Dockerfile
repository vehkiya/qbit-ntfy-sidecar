# Build Stage
FROM golang:1.26.0-alpine AS builder
WORKDIR /app
COPY go.mod ./
# COPY go.sum ./ # Uncomment if using dependencies
RUN go mod download
COPY main.go .
# -ldflags="-w -s" strips debug info for smaller binary
RUN CGO_ENABLED=0 go build -ldflags="-w -s" -o sidecar main.go

# Runtime Stage
# "static-debian12" includes root CA certs needed for HTTPS (ntfy.sh)
FROM gcr.io/distroless/static-debian12
COPY --from=builder /app/sidecar /sidecar
CMD ["/sidecar"]
