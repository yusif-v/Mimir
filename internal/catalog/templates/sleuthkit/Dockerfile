FROM ubuntu:24.04

LABEL maintainer="Mimir Project"
LABEL purpose="Disk forensic analysis with The Sleuth Kit"

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
    sleuthkit \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# Sleuthkit provides: fls, fsstat, icat, mmls, blkcalc, istat, blkls, blkcat,
# dcat, dstat, ffind, fls, fsstat, hfind, icat, ifind, ils, img_stat, istat,
# jcat, jls, mmls, sigfind, sstat

WORKDIR /output
ENTRYPOINT ["fls"]
