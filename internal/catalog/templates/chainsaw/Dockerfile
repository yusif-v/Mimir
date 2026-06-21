FROM alpine:3.20

RUN apk add --no-cache curl && \
    curl -sL "$(curl -s https://api.github.com/repos/WithSecureLabs/chainsaw/releases/latest | grep 'browser_download_url.*musl' | cut -d'"' -f4)" -o /usr/local/bin/chainsaw && \
    chmod +x /usr/local/bin/chainsaw && \
    apk del curl

WORKDIR /output
ENTRYPOINT ["chainsaw"]
