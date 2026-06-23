FROM python:3.12-slim

LABEL maintainer="Mimir Project"
LABEL purpose="Windows registry hive parsing with python-registry"

RUN pip install --no-cache-dir python-registry

WORKDIR /output
ENTRYPOINT ["python", "-m", "registry"]
