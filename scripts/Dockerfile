# This Dockerfile should be used to create an environment to develop
# applications for cozy-stack. It installs couchdb 2 and the cozy-stack.
# It should not be used for hosting your cozy cloud in production.


# Multi-stage image: this step builds cozy-stack
FROM golang:1.11 as build
WORKDIR /go/src/github.com/cozy/cozy-stack

# MailHog
ENV MAILHOG_VERSION 1.0.0
ENV MAILHOG_SRC_URL https://github.com/mailhog/MailHog/releases/download/v$MAILHOG_VERSION/MailHog_linux_amd64
ENV MAILHOG_SRC_SHA256 ba921e04438e176c474d533447ae64707ffcdd1230f0153f86cb188d348f25c0
RUN set -eu; curl -fsSL "$MAILHOG_SRC_URL" -o MailHog \
  && echo "$MAILHOG_SRC_SHA256  MailHog" | sha256sum -c - \
  && chmod +x MailHog

# Build cozy-stack
COPY . .
RUN go get -d -v ./...
RUN ./scripts/build.sh dev


# Multi-stage image: the main image
FROM debian:stretch-slim

# cozy-stack
ENV COZY_STACK_HOST cozy.tools
ENV COZY_STACK_PORT 8080
ENV COZY_STACK_PATH cozy-stack

# CouchDB
ENV COUCHDB_VERSION 2.3.0
ENV COUCHDB_SRC_URL https://dist.apache.org/repos/dist/release/couchdb/source/$COUCHDB_VERSION/apache-couchdb-$COUCHDB_VERSION.tar.gz
ENV COUCHDB_SRC_SHA256 0b3868d042b158d9fd2f504804abd93cd22681c033952f832ce846672c31f352

ARG DEBIAN_FRONTEND=noninteractive

RUN set -eux; apt-get update \
  && apt-get install -y --no-install-recommends \
    build-essential \
    ca-certificates \
    curl \
    libicu57 \
    libmozjs185-1.0 \
    erlang-nox \
    erlang-reltool \
    erlang-dev \
    libicu-dev \
    libmozjs185-dev \
    openssl \
    imagemagick \
    git \
  && rm -rf /var/lib/apt/lists/* \
  && mkdir /usr/src/couchdb \
  && curl -fsSL "$COUCHDB_SRC_URL" -o couchdb.tar.gz \
  && echo "$COUCHDB_SRC_SHA256  couchdb.tar.gz" | sha256sum -c - \
  && tar -xzf couchdb.tar.gz -C /usr/src/couchdb --strip-components=1 \
  && rm couchdb.tar.gz \
  && cd /usr/src/couchdb \
  && ./configure --disable-docs \
  && make release \
  && mv ./rel/couchdb /usr/local \
  && cd / \
  && rm -rf /usr/src/couchdb \
  # Cleanup
  && apt-get purge -y \
    build-essential \
    erlang-dev \
    libicu-dev \
    libmozjs185-dev \
    make \
  && printf "[chttpd]\\nbind_address = 0.0.0.0\\n" \
    > /usr/local/couchdb/etc/local.ini \
  && apt-get autoremove -y && apt-get clean \
  && mkdir -p /data/cozy-app && mkdir -p /data/cozy-storage

ENV PATH $PATH:/usr/local/couchdb/bin

COPY --from=build \
  /go/src/github.com/cozy/cozy-stack/cozy-stack \
  /go/src/github.com/cozy/cozy-stack/scripts/docker-entrypoint.sh \
  /go/src/github.com/cozy/cozy-stack/scripts/cozy-app-dev.sh \
  /go/src/github.com/cozy/cozy-stack/MailHog \
  /usr/bin/

EXPOSE 8080 6060 8025 5984
ENTRYPOINT ["/usr/bin/docker-entrypoint.sh"]
