FROM ubuntu:24.04

LABEL maintainer="Mimir Project"
LABEL purpose="Pattern matching with YARA"

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y yara \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

WORKDIR /output
ENTRYPOINT ["yara"]
