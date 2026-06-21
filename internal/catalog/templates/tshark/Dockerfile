FROM alpine:3.22

LABEL maintainer="Mimir Project"
LABEL purpose="CLI packet analysis with tshark"

RUN apk add --no-cache tshark

WORKDIR /output
ENTRYPOINT ["tshark"]
