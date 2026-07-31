# Cuckoo Sandbox Docker Template for Mimir

## Description
This template provides a Dockerized Cuckoo Sandbox for automated malware analysis within Mimir cases.

## Usage
```
# From within Mimir REPL
sandbox run <sandbox_id> -- cuckoo submit <file>
sandbox run <sandbox_id> -- cuckoo process
```

Or use the cuckoo command directly:
```
cuckoo submit <file>
cuckoo process
```

## Features
- Automated malware analysis
- Behavioral monitoring
- Network traffic capture (PCAP)
- Memory dumps
- Screenshots during execution
- Comprehensive reporting
- Web interface available on port 8090

## Configuration
The template expects:
- Cuckoo installed and configured within the container
- Proper mounting of analysis directory
- Access to submission directory for file analysis

## Testing
To test this template locally:
```bash
# Build the image
docker build -t mimir-cuckoo:test .

# Run a basic test
docker run --rm mimir-cuckoo:test --help

# For full functionality, you would need to:
# 1. Create a cuckoo configuration
# 2. Mount necessary directories for storage
# 3. Submit samples for analysis
```

## Notes
- Built from `blacktop/cuckoo:latest` which provides a stable Cuckoo Sandbox installation
- Uses Alpine Linux package manager (apk) for compatibility
- Includes tcpdump for enhanced network capture capabilities
- Runs as non-root user for security
- Web interface accessible on port 8090 when container is run with `-p 8090:8090`
- Requires proper volume mounting for persistent storage:
  - Evidence/files to analyze: mounted read-only to `/mnt/evidence`
  - Output/storage: mounted read-write to `/home/cuckoo/.cuckoo/storage`