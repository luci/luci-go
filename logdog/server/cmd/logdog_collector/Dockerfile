# Dockerfile extending the generic Go image with application files for a
# single application.
FROM golang:1.9-alpine

ENV mainpkg "go.chromium.org/luci/logdog/server/cmd/logdog_collector"

# Install additional system packages.
RUN apk add --update curl bash && \
    rm -rf /var/cache/apk/*

# Copy the local package files to the container's workspace.
ADD _gopath/src/ /go/src
COPY run.sh /opt/logdog/collector/run.sh

# Build the command inside the container.
# (You may fetch or manage dependencies here,
# either manually or with a tool like "godep".)
RUN go install "${mainpkg}"

# Run the output command by default when the container starts.
ENTRYPOINT ["/bin/bash", "/opt/logdog/collector/run.sh", "/go/bin/logdog_collector"]
