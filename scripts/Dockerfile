# This Dockerfile should be used to create an development environment to
# develop applications for cozy-stack. It installs go and couchdb 2.
# It should not be used for hosting your cozy cloud in production.

FROM debian:jessie

# CouchDB and CGo dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    bash \
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
ENV COUCHDB_VERSION 2.0.0
ENV COUCHDB_SRC_URL https://dist.apache.org/repos/dist/release/couchdb/source/2.0.0/apache-couchdb-$COUCHDB_VERSION.tar.gz
ENV COUCHDB_SRC_SHA256 ccaf3ce9cb06c50a73e091696e557e2a57c5ba02c5b299e1ac2f5b959ee96eca
RUN mkdir /usr/src/couchdb \
  && curl -fsSL "$COUCHDB_SRC_URL" -o couchdb.tar.gz \
  && echo "$COUCHDB_SRC_SHA256  couchdb.tar.gz" | sha256sum -c - \
  && tar -xzf couchdb.tar.gz -C /usr/src/couchdb --strip-components=1 \
  && rm couchdb.tar.gz \
  && cd /usr/src/couchdb \
  && ./configure --disable-docs \
  && make release \
  && mv ./rel/couchdb /usr/local \
  && rm -rf /usr/src/couchdb
ENV PATH $PATH:/usr/local/couchdb/bin


# Golang
ENV GOLANG_VERSION 1.7.4
ENV GOLANG_DOWNLOAD_URL https://golang.org/dl/go$GOLANG_VERSION.linux-amd64.tar.gz
ENV GOLANG_DOWNLOAD_SHA256 47fda42e46b4c3ec93fa5d4d4cc6a748aa3f9411a2a2b7e08e3a6d80d753ec8b
RUN curl -fsSL "$GOLANG_DOWNLOAD_URL" -o golang.tar.gz \
  && echo "$GOLANG_DOWNLOAD_SHA256  golang.tar.gz" | sha256sum -c - \
  && tar -xzf golang.tar.gz -C /usr/local \
  && rm golang.tar.gz

ENV GOPATH /go
ENV PATH $GOPATH/bin:/usr/local/go/bin:$PATH

RUN mkdir -p "$GOPATH/src" "$GOPATH/bin" && chmod -R 777 "$GOPATH"

RUN go get github.com/mholt/caddy/caddy
RUN go get github.com/cozy/cozy-stack


# Cleanup
RUN apt-get purge -y \
    erlang-dev \
    libicu-dev \
    libmozjs185-dev \
  && apt-get autoremove -y \
  && apt-get clean

RUN echo "[chttpd]\nbind_address = 0.0.0.0" \
  > /usr/local/couchdb/etc/local.ini

COPY cozy-app-dev.sh /usr/bin
RUN chmod +x /usr/bin/cozy-app-dev.sh

ENV COZY_PROXY_HOST cozy.local
ENV COZY_PROXY_PORT 8080
ENV COZY_STACK_HOST localhost
ENV COZY_STACK_PORT 8081

RUN mkdir -p /data/cozy-app && mkdir -p /data/cozy-storage

EXPOSE 8080 5984

CMD couchdb 2>/dev/null 1>/dev/null & \
    echo "127.0.0.1 cozy.local app.cozy.local" >> /etc/hosts && \
    /usr/bin/cozy-app-dev.sh \
      -d /data/cozy-app \
      -f /data/cozy-storage
