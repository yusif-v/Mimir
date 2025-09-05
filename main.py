# import virusTotal
# import malwareBazaar
# import threatFox
import os

def env_check():
    env = ".env"
    if not os.path.exists(env):
        print(".env file not found.")
        return
    else:
        print(".env is exist")

env_check()
