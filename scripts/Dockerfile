# This Dockerfile should be used to create an environment to develop
# applications for cozy-stack. It installs couchdb 2 and the cozy-stack.
# It should not be used for hosting your cozy cloud in production.


# Multi-stage image: this step builds cozy-stack (and mailhog)
FROM golang:1.14 as build
WORKDIR /app

# MailHog
ENV MAILHOG_SRC_URL=https://github.com/mailhog/MailHog/releases/download/v1.0.0/MailHog_linux_amd64 \
	MAILHOG_SRC_SHA256=ba921e04438e176c474d533447ae64707ffcdd1230f0153f86cb188d348f25c0
RUN set -eu; curl -fsSL "$MAILHOG_SRC_URL" -o MailHog \
  && echo "$MAILHOG_SRC_SHA256  MailHog" | sha256sum -c - \
  && chmod +x MailHog

# Use docker layer caching to avoid redownloading go modules if the code has
# changed but not the dependencies.
COPY go.mod .
COPY go.sum .
RUN go mod download

# Build cozy-stack
COPY . .
RUN ./scripts/build.sh dev


# Multi-stage image: the main image
FROM debian:stretch-slim

# cozy-stack
ENV COZY_STACK_HOST=cozy.tools \
	COZY_STACK_PORT=8080 \
	COZY_STACK_PATH=cozy-stack \
	COUCHDB_SRC_URL=https://dist.apache.org/repos/dist/release/couchdb/source/2.3.1/apache-couchdb-2.3.1.tar.gz \
	COUCHDB_SRC_SHA256=43eb8cec41eb52871bf22d35f3e2c2ce5b806ebdbce3594cf6b0438f2534227d \
	PATH="$PATH:/usr/local/couchdb/bin"

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
  && curl -fsSL --compressed https://nodejs.org/dist/v12.9.1/node-v12.9.1-linux-x64.tar.xz \
  && tar -xJf "node-v12.9.1-linux-x64.tar.xz" -C /usr/local --strip-components=1 --no-same-owner \
  && rm "node-v12.9.1-linux-x64.tar.xz" \
  && ln -s /usr/local/bin/node /usr/local/bin/nodejs \
  && node --version \
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


COPY --from=build \
  /app/cozy-stack \
  /app/scripts/docker-entrypoint.sh \
  /app/scripts/cozy-app-dev.sh \
  /app/scripts/konnector-node-run.sh \
  /app/MailHog \
  /usr/bin/

EXPOSE 8080 6060 8025 5984
ENTRYPOINT ["/usr/bin/docker-entrypoint.sh"]
