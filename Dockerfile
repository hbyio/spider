# https://docs.docker.com/engine/reference/builder/
############################
# STEP 1 build executable binary
############################
FROM golang:1.15-alpine as builder
# Install git + SSL ca certificates.
# Git is required for fetching the dependencies.
# Ca-certificates is required to call HTTPS endpoints.
RUN apk update && apk add --no-cache git ca-certificates tzdata && update-ca-certificates

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

# Install pg_dump
RUN apk update && apk add postgresql-client

COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/group /etc/group

# Copy our static executable
COPY --from=builder /go/bin/spiderhouse /go/bin/spiderhouse


RUN touch /var/log/spiderhouse.log
RUN chown appuser:appuser /var/log/spiderhouse.log
RUN chmod 766  /var/log/spiderhouse.log

RUN touch /etc/periodic/15min/gospider
RUN chmod a+x /etc/periodic/15min/gospider
RUN echo '#!/bin/sh' >> /etc/periodic/15min/gospider
RUN echo '/go/bin/spiderhouse capture >> /var/log/spiderhouse.log' >> /etc/periodic/15min/gospider

# Use an unprivileged user.
USER appuser:appuser
# Port on which the service will be exposed. Expose on port > 1024
EXPOSE 8080
# Run the spiderhouse binary.
ENTRYPOINT ["/go/bin/spiderhouse", "server"]
