FROM golang:alpine AS builder

RUN apk add --no-cache git
WORKDIR /app
COPY . .
RUN go build -ldflags="-s -w" -o /yamanaka-server .

# No build artifacts for final stage (git still needed)
FROM alpine:latest

RUN apk add --no-cache git
WORKDIR /app
RUN mkdir -p /app/data
COPY --from=builder /yamanaka-server .
EXPOSE 8080

# The command to run when the container starts
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
CMD ["/app/yamanaka-server"]
