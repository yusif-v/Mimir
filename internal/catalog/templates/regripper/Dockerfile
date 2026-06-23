FROM ubuntu:24.04

LABEL maintainer="Mimir Project"
LABEL purpose="Windows registry hive analysis with RegRipper"

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
    perl \
    libparse-win32registry-perl \
    wget \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# Install RegRipper from GitHub
RUN wget -q https://github.com/keydet89/RegRipper3.0/archive/refs/heads/master.tar.gz -O /tmp/regripper.tar.gz && \
    tar -xzf /tmp/regripper.tar.gz -C /opt/ && \
    mv /opt/RegRipper3.0-master /opt/regripper && \
    rm /tmp/regripper.tar.gz

WORKDIR /output
ENTRYPOINT ["perl", "/opt/regripper/rip.pl"]
