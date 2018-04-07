# Start from a Debian image with the latest version of Go installed
# and a workspace (GOPATH) configured at /go.
FROM alpine:3.6

ARG GSNOVA_VER
ENV GSNOVA_VER ${GSNOVA_VER:-latest}

RUN \
    apk add --no-cache --virtual  curl \
    && mkdir -m 777 /gsnova \
    && cd /gsnova \
    && curl -fSL https://github.com/yinqiwen/gsnova/releases/download/$GSNOVA_VER/gsnova_linux_amd64-$GSNOVA_VER.tar.bz2 | tar xj  \
    && rm -rf gsnova_linux_amd64-$GSNOVA_VER.tar.bz2 \
    && chgrp -R 0 /gsnova \
    && chmod -R g+rwX /gsnova 

#WORKDIR /go/bin
# Run the outyet command by default when the container starts.
CMD ["/gsnova/gsnova","-cmd" ,"-server", "-key", "809240d3a021449f6e67aa73221d42df942a308a", "-listen", "tcp://:9443", "-listen", "quic://:9443", "-listen", "http://:9444", "-listen", "kcp://:9444", "-listen", "http2://:9445", "-listen", "tls://:9446", "-log", "console"]
# Document that the service listens on port 9443.
EXPOSE 9443 9444 9445 9446
