# Build stage
FROM golang:1.26.2-alpine@sha256:d826a798b3c959dcfb1807661ceb3856b3e8e19e763137cc3542472d82935cf3 AS builder

ENV GOTOOLCHAIN=auto

WORKDIR /app

RUN apk add --no-cache nodejs npm

# Copy go mod and sum files
COPY go.mod ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copy the source from the current directory to the Working Directory inside the container
COPY . .

# Build dashboard2 frontend assets before embedding in Go binary
# Build dashboard2 frontend assets before embedding in Go binary
# If the SPA was pre-built (e.g. via `make ui-build` before `make demo`), skip the npm build
# to avoid OOM inside the container. Fall back to building in-container if assets are absent.
RUN if [ ! -f dashboard/static/spa/index.html ]; then \
        npm --prefix ui ci --include=optional && \
        NODE_OPTIONS="--max-old-space-size=4096" npm --prefix ui run build; \
    fi

# Build the Go app
RUN go build \
    -tags 'netgo osusergo static_build' \
    -trimpath \
    -ldflags="-w -s -extldflags '-static -fno-PIC'" \
    -o gopherstack

# Final stage
FROM scratch

WORKDIR /root/

# Copy the Pre-built binary from the previous stage
COPY --from=builder /app/gopherstack .

# Expose port 8000 to the outside world
EXPOSE 8000

# Expose MQTT broker port
EXPOSE 1883

# Expose Azure Blob/Queue/Table (Azurite-compatible), Service Bus, and Cosmos DB ports
EXPOSE 10000 10001 10002 10003 8081

# OCI label pointing to the source repository
LABEL org.opencontainers.image.source="https://github.com/blackbirdworks/gopherstack"

# Command to run the executable
CMD ["./gopherstack"]
