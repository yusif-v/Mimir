# import virusTotal
# import malwareBazaar
# import threatFox
import os

def env_check():
    env = ".env"
    keys = ["OTX_API_KEY", "ABUSE_API_KEY", "ACH_API_KEY", "VT_API_KEY"]

    if not os.path.exists(env):
        print(".env file not found. You need to create an .env file.")
        return False

    with open(env, "r") as f:
        content = f.read()

    envstatus = True

    for key in keys:
        if f"{key}=" in content:
            print(f"✅ {key} is ready")
        else:
            print(f"⚠️ {key} is missing")
            envstatus = False

    return envstatus

if __name__ == "__main__":
    print(env_check())