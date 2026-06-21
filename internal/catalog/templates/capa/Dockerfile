FROM alpine:3.20

RUN apk add --no-cache curl && \
    curl -sL "$(curl -s https://api.github.com/repos/mandiant/capa/releases/latest | grep 'browser_download_url.*linux' | cut -d'"' -f4)" -o /usr/local/bin/capa && \
    chmod +x /usr/local/bin/capa && \
    apk del curl

WORKDIR /output
ENTRYPOINT ["capa"]
