# Use Go 1.24 (latest stable) instead of 1.21
FROM golang:1.24-alpine AS build
WORKDIR /app

# Copy go.mod first for caching
COPY go.mod ./

# Download dependencies (will generate go.sum if needed)
RUN go mod tidy

# Copy the rest of the source
COPY . .

# Build binary
RUN go build -o stremio-local main.go

# Final lightweight image
FROM alpine:latest
WORKDIR /media
COPY --from=build /app/stremio-local /usr/local/bin/stremio-local
EXPOSE 8081
ENTRYPOINT ["/usr/local/bin/stremio-local"]
