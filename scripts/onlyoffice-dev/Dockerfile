# Taken from https://github.com/ONLYOFFICE/Docker-DocumentServer/blob/master/Dockerfile

FROM ubuntu:22.04

ENV LANG=en_US.UTF-8 LANGUAGE=en_US:en LC_ALL=en_US.UTF-8 \
    DEBIAN_FRONTEND=noninteractive \
	PG_VERSION=12

COPY . /usr/bin/

RUN echo "#!/bin/sh\nexit 0" > /usr/sbin/policy-rc.d && \
    apt-get -y update && \
    apt-get -yq install wget apt-utils apt-transport-https gnupg locales && \
    locale-gen en_US.UTF-8 && \
    echo ttf-mscorefonts-installer msttcorefonts/accepted-mscorefonts-eula select true | debconf-set-selections && \
    apt-get -yq install --no-install-recommends \
        adduser \
        bomstrip \
        certbot \
        curl \
        gconf-service \
        htop \
        libasound2 \
        libboost-regex-dev \
        libcairo2 \
        libcurl3-gnutls \
        libcurl4 \
        libgtk-3-0 \
        libnspr4 \
        libnss3 \
        libstdc++6 \
        libxml2 \
        libxss1 \
        libxtst6 \
        mysql-client \
        nano \
        net-tools \
        netcat-openbsd \
        postgresql \
        postgresql-client \
        pwgen \
        rabbitmq-server \
        software-properties-common \
        sudo \
        ttf-mscorefonts-installer \
        xvfb \
        zlib1g && \
    if [  $(ls -l /usr/share/fonts/truetype/msttcorefonts | wc -l) -ne 61 ]; \
        then echo 'msttcorefonts failed to download'; exit 1; fi  && \
    echo "SERVER_ADDITIONAL_ERL_ARGS=\"+S 1:1\"" | tee -a /etc/rabbitmq/rabbitmq-env.conf && \
    pg_conftool $PG_VERSION main set listen_addresses 'localhost' && \
    service postgresql restart && \
    sudo -u postgres psql -c "CREATE DATABASE onlyoffice;" && \
    sudo -u postgres psql -c "CREATE USER onlyoffice WITH password 'onlyoffice';" && \
    sudo -u postgres psql -c "GRANT ALL privileges ON DATABASE onlyoffice TO onlyoffice;" && \ 
    echo "deb http://download.onlyoffice.com/repo/debian squeeze main" | tee /etc/apt/sources.list.d/ds.list && \
    apt-key adv --keyserver keyserver.ubuntu.com --recv-keys 0x8320ca65cb2de8e5 && \
    apt-get -y update && \
    apt-get -yq install onlyoffice-documentserver && \
	cp /usr/bin/local.json /etc/onlyoffice/documentserver/local.json && \
	service postgresql stop && \
	service rabbitmq-server stop && \
	apt-get clean && \
    rm -rf /var/log/onlyoffice && \
    rm -rf /var/lib/apt/lists/* /var/cache/apt

EXPOSE 8000
ENTRYPOINT ["/usr/bin/docker-entrypoint.sh"]
