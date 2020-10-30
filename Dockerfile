# https://docs.docker.com/engine/reference/builder/
############################
# STEP 1 build executable binary
############################
FROM golang:1.15-alpine as builder
# Install git + SSL ca certificates.
# Git is required for fetching the dependencies.
# Ca-certificates is required to call HTTPS endpoints.
RUN apk update && apk add --no-cache git ca-certificates tzdata postgresql-client && update-ca-certificates

# Create appuser
ENV USER=appuser
ENV UID=10001
## See https://stackoverflow.com/a/55757473/12429735RUN
RUN adduser --disabled-password --gecos "" --home "/nonexistent" --shell "/sbin/nologin" --no-create-home --uid "${UID}" "${USER}"

WORKDIR /src
COPY . .
# Fetch dependencies.
# Using go mod with go 1.11
RUN go mod download
RUN go mod verify
# Build the binary
RUN GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /go/bin/spiderhouse
############################
# STEP 2 build a small image
############################
FROM alpine:latest
# Import from builder.
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/group /etc/group
COPY --from=builder /usr/bin/pg_dump /usr/bin/pg_dump
# Copy our static executable
COPY --from=builder /go/bin/spiderhouse /go/bin/spiderhouse
# Use an unprivileged user.
USER appuser:appuser
# Port on which the service will be exposed. Expose on port > 1024
EXPOSE 8080
# Run the spiderhouse binary.
ENTRYPOINT ["/go/bin/spiderhouse", "server"]