# Build stage
FROM golang:1.26-bookworm AS builder

WORKDIR /app

# Retrieve application dependencies.
# This allows the container build to reuse cached dependencies.
COPY go.mod ./
# If there are dependencies, uncomment the next line
# COPY go.sum ./
# RUN go mod download

# Copy local code to the container image.
COPY . ./

# Build the binary.
# -ldflags="-w -s" reduces binary size.
# CGO_ENABLED=0 builds a statically-linked binary.
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o server ./cmd/app

# Run stage
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /

# Copy the binary from the builder stage.
COPY --from=builder /app/server /server

# Use the non-root user.
USER nonroot:nonroot

# Run the web service on container startup.
ENTRYPOINT ["/server"]
