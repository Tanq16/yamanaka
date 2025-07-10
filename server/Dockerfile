# --- Build Stage ---
# Use an official Golang runtime as a parent image
FROM golang:alpine AS builder

# Install git, which is required for the application to run
# It's included here to ensure all build tools are present
RUN apk add --no-cache git

# Set the working directory inside the container
WORKDIR /app

# Copy the rest of the source code
COPY . .

# Build the Go application, creating a static binary
# -s and -w flags reduce the size of the binary
RUN go build -ldflags="-s -w" -o /yamanaka-server .

# --- Final Stage ---
# Use a minimal alpine image for the final container to keep it small
FROM alpine:latest

# Install git, which is a runtime dependency for the server
RUN apk add --no-cache git

# Set the working directory
WORKDIR /app

# Create the data directory for the vault storage
RUN mkdir -p /app/data

# Copy the compiled binary from the builder stage
COPY --from=builder /yamanaka-server .

# Expose port 8080 to the outside world
EXPOSE 8080

# The command to run when the container starts
CMD ["/app/yamanaka-server"]
