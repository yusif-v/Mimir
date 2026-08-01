FROM debian:bookworm-slim

ARG TARGETARCH

RUN apt-get update && apt-get install -y --no-install-recommends \
        curl ca-certificates tar \
    && rm -rf /var/lib/apt/lists/*

# chainsaw releases ship separate assets per target (x86_64 / aarch64
# unknown-linux-gnu). There is no musl asset, so glibc (debian) is required.
# TARGETARCH (BuildKit) maps amd64->x86_64 and arm64->aarch64 so the image is
# native on both Apple Silicon Macs and amd64 servers.
RUN asset=x86_64; [ "$TARGETARCH" = "arm64" ] && asset=aarch64; \
    curl -sL "$(curl -s https://api.github.com/repos/WithSecureLabs/chainsaw/releases/latest | grep "browser_download_url.*${asset}-unknown-linux-gnu.tar.gz" | cut -d'"' -f4)" -o /tmp/chainsaw.tar.gz \
    && tar -xzf /tmp/chainsaw.tar.gz -C /tmp \
    && install -m 0755 /tmp/chainsaw/chainsaw /usr/local/bin/chainsaw \
    && rm -rf /tmp/chainsaw /tmp/chainsaw.tar.gz \
    && apt-get purge -y --auto-remove curl \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /output
ENTRYPOINT ["chainsaw"]
