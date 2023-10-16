# This Dockerfile should be used to create an environment to develop
# applications for cozy-stack. It installs couchdb 2 and the cozy-stack.
# It should not be used for hosting your cozy cloud in production.


# Multi-stage image: this step builds cozy-stack (and mailhog)
FROM golang:1.21-bullseye as build
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
FROM debian:bullseye-slim

# cozy-stack
ENV COZY_STACK_HOST=cozy.localhost \
	COZY_STACK_PORT=8080 \
	COZY_STACK_PATH=cozy-stack \
	PATH="$PATH:/opt/couchdb/bin"

ARG DEBIAN_FRONTEND=noninteractive

RUN set -eux; apt-get update \
  && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    apt-transport-https \
    gnupg \
    openssl \
    fonts-lato \
    imagemagick \
    git \
  && curl https://couchdb.apache.org/repo/keys.asc | gpg --dearmor > /usr/share/keyrings/couchdb-archive-keyring.gpg \
  && echo "deb [signed-by=/usr/share/keyrings/couchdb-archive-keyring.gpg] https://apache.jfrog.io/artifactory/couchdb-deb/ bullseye main" > /etc/apt/sources.list.d/couchdb.list \
  && curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key | gpg --dearmor > /usr/share/keyrings/nodesource.gpg \
  && echo "deb [signed-by=/usr/share/keyrings/nodesource.gpg] https://deb.nodesource.com/node_16.x nodistro main" > /etc/apt/sources.list.d/nodesource.list \
  && apt update \
  && echo "couchdb couchdb/mode select standalone" | debconf-set-selections \
  && echo "couchdb couchdb/mode seen true" | debconf-set-selections \
  && echo "couchdb couchdb/bindaddress string 0.0.0.0" | debconf-set-selections \
  && echo "couchdb couchdb/bindaddress seen true" | debconf-set-selections \
  && echo "couchdb couchdb/adminpass password password" | debconf-set-selections \
  && echo "couchdb couchdb/adminpass seen true" | debconf-set-selections \
  && echo "couchdb couchdb/adminpass_again password password" | debconf-set-selections \
  && echo "couchdb couchdb/adminpass_again seen true" | debconf-set-selections \
  && echo "couchdb couchdb/cookie string elmo" | debconf-set-selections \
  && echo "couchdb couchdb/cookie seen true" | debconf-set-selections \
  && apt install -y --no-install-recommends couchdb=3.2.2* nodejs \
  && rm -rf /var/lib/apt/lists/* \
  && ln -s /usr/local/bin/node /usr/local/bin/nodejs \
  && node --version \
  # Cleanup
  && apt-get autoremove -y && apt-get clean \
  && mkdir -p /data/cozy-app && mkdir -p /data/cozy-storage


COPY --from=build \
  /app/cozy-stack \
  /app/scripts/docker-entrypoint.sh \
  /app/scripts/cozy-app-dev.sh \
  /app/scripts/konnector-node16-run.sh \
  /app/MailHog \
  /usr/bin/

EXPOSE 8080 6060 8025 5984
ENTRYPOINT ["/usr/bin/docker-entrypoint.sh"]
