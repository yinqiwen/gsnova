# Start from a Debian image with the latest version of Go installed
# and a workspace (GOPATH) configured at /go.
FROM golang:alpine

# Copy the local package files to the container's workspace.
ADD ./server.json /go/bin

RUN apk add --update git
# Build the outyet command inside the container.
# (You may fetch or manage dependencies here,
# either manually or with a tool like "godep".)
WORKDIR /go/src/github.com/yinqiwen
RUN git clone -b master https://github.com/yinqiwen/gsnova/
WORKDIR /go/src/github.com/yinqiwen/gsnova/remote/vps
RUN go get ./...
RUN go install github.com/yinqiwen/gsnova/remote/vps

WORKDIR /go/bin
# Run the outyet command by default when the container starts.
ENTRYPOINT /go/bin/vps

# Document that the service listens on port 9443.
EXPOSE 9443