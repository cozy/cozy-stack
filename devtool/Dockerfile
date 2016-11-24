FROM debian:jessie

ENV COUCHDB_VERSION 2.0.0
ENV COUCHDB_SRC_URL https://dist.apache.org/repos/dist/release/couchdb/source/2.0.0/apache-couchdb-$COUCHDB_VERSION.tar.gz
ENV COUCHDB_SRC_SHA256 ccaf3ce9cb06c50a73e091696e557e2a57c5ba02c5b299e1ac2f5b959ee96eca

ENV GOLANG_VERSION 1.7.3
ENV GOLANG_DOWNLOAD_URL https://golang.org/dl/go$GOLANG_VERSION.linux-amd64.tar.gz
ENV GOLANG_DOWNLOAD_SHA256 508028aac0654e993564b6e2014bf2d4a9751e3b286661b0b0040046cf18028e

# CouchDB and CGo dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    ca-certificates \
    curl \
    libicu52 \
    libmozjs185-1.0 \
    erlang \
    libicu-dev \
    libmozjs185-dev \
    openssl \
    g++ \
    gcc \
    libc6-dev \
    make \
    pkg-config \
  && rm -rf /var/lib/apt/lists/*

# CouchDB
RUN mkdir /usr/src/couchdb \
  && curl -fsSL "$COUCHDB_SRC_URL" -o couchdb.tar.gz \
  && echo "$COUCHDB_SRC_SHA256  couchdb.tar.gz" | sha256sum -c - \
  && tar -xzf couchdb.tar.gz -C /usr/src/couchdb --strip-components=1 \
  && rm couchdb.tar.gz \
  && cd /usr/src/couchdb \
  && ./configure --disable-docs \
  && make release \
  && mv ./rel/couchdb /usr/local

ENV PATH $PATH:/usr/local/couchdb/bin

# Go
RUN curl -fsSL "$GOLANG_DOWNLOAD_URL" -o golang.tar.gz \
  && echo "$GOLANG_DOWNLOAD_SHA256  golang.tar.gz" | sha256sum -c - \
  && tar -xzf golang.tar.gz -C /usr/local \
  && rm golang.tar.gz

ENV GOPATH /go
ENV PATH $GOPATH/bin:/usr/local/go/bin:$PATH

RUN mkdir -p "$GOPATH/src" "$GOPATH/bin" && chmod -R 777 "$GOPATH"

RUN go get github.com/mholt/caddy/caddy \
  && go get github.com/cozy/cozy-stack

# Cleanup
RUN apt-get purge -y \
    erlang-dev \
    libicu-dev \
    libmozjs185-dev \
  && apt-get autoremove -y \
  && apt-get clean \
  && rm -rf /usr/src/couchdb

COPY dev.sh /usr/bin
RUN chmod +x /usr/bin/dev.sh

ENV COZY_DEV_HOST cozy.local
ENV COZY_DEV_PORT 8080
ENV COZY_STACK_HOST localhost
ENV COZY_STACK_PORT 8081
ENV COUCHDB_ENABLE 1
ENV COUCHDB_HOST localhost
ENV COUCHDB_PORT 5984

RUN mkdir -p /data/cozy-app

ENTRYPOINT ["/usr/bin/dev.sh"]
CMD ["-d /data/cozy-app"]
