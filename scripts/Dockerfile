# This Dockerfile should be used to create an environment to develop
# applications for cozy-stack. It installs couchdb 2 and the cozy-stack.
# It should not be used for hosting your cozy cloud in production.

FROM debian:jessie


# CouchDB
ENV COUCHDB_VERSION 2.0.0
ENV COUCHDB_SRC_URL https://dist.apache.org/repos/dist/release/couchdb/source/$COUCHDB_VERSION/apache-couchdb-$COUCHDB_VERSION.tar.gz
ENV COUCHDB_SRC_SHA256 ccaf3ce9cb06c50a73e091696e557e2a57c5ba02c5b299e1ac2f5b959ee96eca

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    ca-certificates \
    curl \
    libicu52 \
    libmozjs185-1.0 \
    erlang-nox \
    erlang-reltool \
    erlang-dev \
    libicu-dev \
    libmozjs185-dev \
    openssl \
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
  && rm -rf /usr/src/couchdb \
  # Cleanup
  && apt-get purge -y \
    binutils \
    build-essential \
    cpp \
    erlang-dev \
    libicu-dev \
    libmozjs185-dev \
    make \
    perl \
  && echo "[chttpd]\nbind_address = 0.0.0.0" \
    > /usr/local/couchdb/etc/local.ini \
  && apt-get autoremove -y && apt-get clean \
  && apt-get install -y libicu52 --no-install-recommends

ENV PATH $PATH:/usr/local/couchdb/bin

# MailHog
ENV MAILHOG_VERSION 0.2.1
ENV MAILHOG_SRC_URL https://github.com/mailhog/MailHog/releases/download/v$MAILHOG_VERSION/MailHog_linux_amd64
ENV MAILHOG_SRC_SHA256 25b3cd56e7a1042b695fed8b32808eef778eafa8fb6bd583dde90a8fc2877e0e

# cozy-stack
ENV COZY_STACK_HOST cozy.tools
ENV COZY_STACK_PORT 8080
ENV COZY_STACK_PATH cozy-stack

COPY cozy-app-dev.sh cozy-stack /usr/bin/
RUN curl -fsSL "$MAILHOG_SRC_URL" -o /usr/bin/MailHog \
  && echo "$MAILHOG_SRC_SHA256  /usr/bin/MailHog" | sha256sum -c - \
  && chmod +x /usr/bin/cozy-app-dev.sh /usr/bin/cozy-stack /usr/bin/MailHog \
  && mkdir -p /data/cozy-app && mkdir -p /data/cozy-storage

EXPOSE 8080 8025 5984

CMD couchdb 2>/dev/null 1>/dev/null & \
    MailHog 2>/dev/null 1>/dev/null & \
    echo "0.0.0.0 cozy.tools" >> /etc/hosts && \
    /usr/bin/cozy-app-dev.sh \
      -d /data/cozy-app \
      -f /data/cozy-storage
