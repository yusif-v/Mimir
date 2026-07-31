FROM blacktop/cuckoo:latest

# Install additional dependencies if needed
RUN apk add --no-cache \
    tcpdump \
    && rm -rf /var/cache/apk/*

# Set working directory
WORKDIR /home/cuckoo

# Default command
ENTRYPOINT ["cuckoo"]