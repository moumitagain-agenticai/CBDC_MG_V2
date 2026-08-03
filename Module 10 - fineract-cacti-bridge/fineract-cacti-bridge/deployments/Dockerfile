# ---- build stage ----
FROM golang:1.21-alpine AS build
WORKDIR /src

# Cache dependencies first.
COPY go.mod go.sum* ./
RUN go mod download || true

# Build.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /out/cacti-bridge ./cmd/server

# ---- runtime stage ----
FROM alpine:3.19
RUN apk add --no-cache ca-certificates && \
    addgroup -S app && adduser -S -G app app
WORKDIR /app

COPY --from=build /out/cacti-bridge /app/cacti-bridge
COPY configs/config.example.yaml /app/configs/config.yaml

USER app
EXPOSE 8080
ENTRYPOINT ["/app/cacti-bridge"]
CMD ["-config", "/app/configs/config.yaml"]
