FROM debian:bookworm-slim

ARG TARGETARCH

RUN apt-get update && apt-get install -y --no-install-recommends \
        curl ca-certificates unzip \
    && rm -rf /var/lib/apt/lists/*

# capa releases ship per-target zips: `-linux` (x86_64) and `-linux-arm64`
# (plus -linux-py312 / macos / windows). The standalone binary is glibc-linked,
# so a glibc base (debian) is required — alpine/musl cannot run it. TARGETARCH
# (BuildKit) selects the native asset: amd64->linux, arm64->linux-arm64.
RUN asset=linux; [ "$TARGETARCH" = "arm64" ] && asset=linux-arm64; \
    curl -sL "$(curl -s https://api.github.com/repos/mandiant/capa/releases/latest | grep "browser_download_url.*capa-v.*-${asset}.zip" | cut -d'"' -f4)" -o /tmp/capa.zip \
    && unzip -q /tmp/capa.zip -d /tmp/capa \
    && install -m 0755 /tmp/capa/capa /usr/local/bin/capa \
    && rm -rf /tmp/capa /tmp/capa.zip \
    && apt-get purge -y --auto-remove curl \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /output
ENTRYPOINT ["capa"]
