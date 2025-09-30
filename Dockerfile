# Use official Go image
FROM golang:1.21-alpine AS build

# Set working directory
WORKDIR /app

# Copy go.mod first for caching
COPY go.mod ./
RUN go mod download

# Copy the rest of the source
COPY . .

# Build the binary
RUN go build -o stremio-local main.go

# --- Final minimal image ---
FROM alpine:latest

# Create a media folder
RUN mkdir -p /media

# Copy the binary
COPY --from=build /app/stremio-local /usr/local/bin/stremio-local

# Expose the port
EXPOSE 8081

# Set the entrypoint
ENTRYPOINT ["/usr/local/bin/stremio-local"]

# Default working directory
WORKDIR /media
