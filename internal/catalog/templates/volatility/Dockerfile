FROM ubuntu:24.04

LABEL maintainer="Mimir Project"
LABEL purpose="Memory forensics with Volatility 3"

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
    python3 python3-pip python3-venv \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# Install Volatility 3 from PyPI. The base package pulls only its required
# dependencies (pefile, yara-python, capstone, pycryptodome) as prebuilt
# wheels — no compiler needed. We deliberately avoid the [full] extra, which
# drags in leechcorepyc: an FPGA / live-memory-acquisition driver that needs
# libusb + a C toolchain and is useless for analyzing memory-dump files.
RUN python3 -m venv /opt/venv && \
    /opt/venv/bin/pip install --no-cache-dir volatility3

ENV PATH="/opt/venv/bin:${PATH}"
WORKDIR /output
ENTRYPOINT ["vol"]
