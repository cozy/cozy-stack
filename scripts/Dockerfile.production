FROM golang:1.11-stretch AS builder

ARG BUILD=release

WORKDIR /go/src/github.com/cozy/cozy-stack
COPY . .
RUN set -eux && \
    go get -d -v ./... && \
    bash -ex ./scripts/build.sh ${BUILD} && \
    rm ./cozy-stack*.sha256



FROM node:8-stretch-slim

ENV COUCHDB_PROTOCOL=http \
    COUCHDB_HOST=couchdb \
    COUCHDB_PORT=5984 \
    COUCHDB_USER=cozy \
    COUCHDB_PASSWORD=cozy

COPY --from=builder /go/src/github.com/cozy/cozy-stack/cozy-stack* /usr/local/bin/cozy-stack
COPY ./scripts/docker-entrypoint-production.sh /usr/local/bin/docker-entrypoint.sh
COPY ./scripts/wait-for-it.sh ./scripts/konnector-*-run.sh /usr/local/bin/
COPY ./scripts/cozy.yaml /etc/cozy/

RUN set -eux && \
    apt-get update && \
    apt-get upgrade -y && \
    apt-get install -y imagemagick ca-certificates curl gosu git && \
    gosu nobody true && \
    apt-get clean && \
    rm -rf /tmp/* /var/tmp && \
    chmod +x /usr/local/bin/*.sh

WORKDIR /var/lib/cozy

VOLUME /var/lib/cozy/data

EXPOSE 6060 8080

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["cozy-stack","serve"]
