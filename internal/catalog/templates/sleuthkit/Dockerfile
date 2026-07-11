FROM ubuntu:24.04

LABEL maintainer="Mimir Project"
LABEL purpose="Disk forensic analysis with The Sleuth Kit"

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
    sleuthkit \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# Sleuthkit provides: fls, fsstat, icat, mmls, blkcalc, istat, blkls, blkcat,
# dcat, dstat, ffind, hfind, icat, ifind, ils, img_stat, istat,
# jcat, jls, mmls, sigfind, sstat

WORKDIR /output
# ENTRYPOINT is a passthrough that exec's whatever binary + args the caller
# provides (e.g. `docker run dfir-sleuthkit:latest icat -r img 0`).
# A bare `["fls"]` ENTRYPOINT would prepend `fls` to every arg,
# making only `fls` invokable.
ENTRYPOINT ["sh", "-c", "exec \"$@\"", "mimir"]
