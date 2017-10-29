# Start from a Debian image with the latest version of Go installed
# and a workspace (GOPATH) configured at /go.
FROM alpine:3.6

ENV VER=0.30.0

RUN \
    apk add --no-cache --virtual  curl \
    && mkdir -m 777 /gsnova \
    && cd /gsnova \
    && curl -fSL https://github.com/yinqiwen/gsnova/releases/download/v$VER/gsnova_server_linux_amd64-v$VER.tar.bz2 | tar xj  \
    && rm -rf gsnova_server_linux_amd64-v$VER.tar.bz2 \
    && chgrp -R 0 /gsnova \
    && chmod -R g+rwX /gsnova 

#WORKDIR /go/bin
# Run the outyet command by default when the container starts.
CMD ["/gsnova/gsnova_server","-cmd" ,"-key", "809240d3a021449f6e67aa73221d42df942a308a", "-tcp", ":9443", "-quic", ":9443", "-http", ":9444", "-kcp", ":9444", "-http2", ":9445", "-tls", ":9446", "-log", "console"]
# Document that the service listens on port 9443.
EXPOSE 9443 9444 9445 9446