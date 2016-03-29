# Dockerfile extending the generic Go image with application files for a
# single application.
FROM golang:1.6

# Copy the local package files to the container's workspace.
ADD _gopath/src/ /go/src
COPY run.sh /opt/logdog_archivist/run.sh

# Build the command inside the container.
# (You may fetch or manage dependencies here,
# either manually or with a tool like "godep".)
RUN go install github.com/luci/luci-go/server/cmd/logdog_archivist

# Run the output command by default when the container starts.
ENTRYPOINT ["/opt/logdog_archivist/run.sh", "/go/bin/logdog_archivist"]
