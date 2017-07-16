# Start from a Debian image with the latest version of Go installed
# and a workspace (GOPATH) configured at /go.
FROM golang:1.8.3-alpine3.6

# Copy the local package files to the container's workspace.
#ADD ./server.json /go/bin

RUN apk add --update git && \
    go get -t github.com/yinqiwen/gsnova/remote/paas  && \
    go install github.com/yinqiwen/gsnova/remote/paas 

#WORKDIR /go/bin
# Run the outyet command by default when the container starts.
ENTRYPOINT ["/go/bin/paas"]
CMD ["-key", "809240d3a021449f6e67aa73221d42df942a308a", "-listen", ":9443"]

# Document that the service listens on port 9443.
EXPOSE 9443