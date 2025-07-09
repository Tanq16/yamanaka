# --- Build Stage ---
# Use an official Golang runtime as a parent image
FROM golang:1.21-alpine AS builder

# Install git, which is required for the application to run
RUN apk add --no-cache git

WORKDIR /app

# Copy the Go module files and download dependencies
# You will need to generate these with `go mod init yamanaka` and `go mod tidy`
COPY go.mod ./
COPY go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build the Go app
# -o /server is the output path for the compiled binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /server .

# --- Final Stage ---
# Use a minimal base image for the final container
FROM alpine:latest

# Add git again, as it's a runtime dependency
RUN apk add --no-cache git

WORKDIR /app

# Copy the compiled binary from the builder stage
COPY --from=builder /server .

# Create the data directory where the vault will be stored
RUN mkdir data

# Expose port 8080 to the outside world
EXPOSE 8080

# Command to run the executable when the container starts
CMD ["/app/server"]
