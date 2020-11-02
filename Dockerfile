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
USER root
# Install pg_dump
RUN apk update && apk add --no-cache postgresql-client openrc busybox-initscripts

COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/group /etc/group

# Copy our static executable
COPY --from=builder /go/bin/spiderhouse /go/bin/spiderhouse

# Add log file
RUN touch /var/log/spiderhouse.log
RUN chown appuser:appuser /var/log/spiderhouse.log
RUN chmod 766  /var/log/spiderhouse.log

# Empty crontab first
RUN cat /dev/null | crontab -

# Add cron jobs
RUN (echo " */1 * * * *  echo \"run for your life\" >> /var/log/spiderhouse.log") | crontab -
RUN (crontab -l ; echo " 0 * * * *  /go/bin/spiderhouse capture --prefix=hourly >> /var/log/spiderhouse.log") | crontab -
RUN (crontab -l ; echo " 0 6 * * *  /go/bin/spiderhouse capture --prefix=daily >> /var/log/spiderhouse.log") | crontab -
RUN (crontab -l ; echo " 0 8 1 * *  /go/bin/spiderhouse capture --prefix=monthly >> /var/log/spiderhouse.log") | crontab -
                       # * * * * *  command to execute
                       # │ │ │ │ │
                       # │ │ │ │ │
                       # │ │ │ │ └───── day of week (0 - 6) (0 to 6 are Sunday to Saturday, or use names; 7 is Sunday, the same as 0)
                       # │ │ │ └────────── month (1 - 12)
                       # │ │ └─────────────── day of month (1 - 31)
                       # │ └──────────────────── hour (0 - 23)
                       # └───────────────────────── min (0 - 59)

# Use an unprivileged user
# Todo : impossible at the moment because cron needs root BUT find a way
# USER appuser:appuser

# Port on which the service will be exposed. Expose on port > 1024
EXPOSE 8080

# Run the spiderhouse cron.
CMD ["crond", "-f", "-d", "8"]