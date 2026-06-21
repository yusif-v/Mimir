FROM python:3.12-alpine

LABEL maintainer="Mimir Project"
LABEL purpose="Office document analysis with oletools"

ENV DEBIAN_FRONTEND=noninteractive

RUN pip install --no-cache-dir oletools

WORKDIR /output
ENTRYPOINT ["olevba"]
