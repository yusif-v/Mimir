import sys
from shell import mimir
from setup import setup

if __name__ == "__main__":
    if setup():
        mimir()
    else:
        sys.exit(1)