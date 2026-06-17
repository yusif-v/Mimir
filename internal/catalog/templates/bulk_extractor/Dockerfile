FROM ubuntu:24.04

LABEL maintainer="Mimir Project"
LABEL purpose="Disk/memory feature carving with bulk_extractor"

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y bulk-extractor \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

WORKDIR /output
ENTRYPOINT ["bulk_extractor"]
