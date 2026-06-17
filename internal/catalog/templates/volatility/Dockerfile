FROM ubuntu:24.04

LABEL maintainer="Mimir Project"
LABEL purpose="Memory forensics with Volatility 3"

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
    python3 python3-pip python3-venv \
    git build-essential libffi-dev libssl-dev \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

RUN git clone https://github.com/volatilityfoundation/volatility3.git /opt/volatility3
WORKDIR /opt/volatility3
RUN python3 -m venv /opt/venv && /opt/venv/bin/pip install --no-cache-dir -e ".[full]"

ENV PATH="/opt/venv/bin:${PATH}"
WORKDIR /output
ENTRYPOINT ["python3", "/opt/volatility3/vol.py"]
