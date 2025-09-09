import os
import sys
from shell import mimir


def env_check():
    env = ".env"
    keys = ["OTX_API_KEY", "ABUSE_API_KEY", "ACH_API_KEY", "VT_API_KEY"]

    if not os.path.exists(env):
        print("No .env file found.")
        print(
            "Please create a `.env` file based on `env.sample` and add your API keys."
        )
        print("Mimir cannot run without at least one API key configured.")
        return False

    with open(env, "r") as f:
        content = f.read()

    missing_keys = []

    for key in keys:
        if f"{key}=" in content:
            print(f"{key} found")
        else:
            print(f"{key} missing")
            missing_keys.append(key)

    if not missing_keys:
        print("All API keys are configured. Mimir is ready to go.")
    elif len(missing_keys) == len(keys):
        print("No API keys configured.")
        print("Please add at least one key from `env.sample` to use Mimir.")
        return False
    else:
        print(f"Some keys are missing: {', '.join(missing_keys)}")
        print("Mimir will run, but related features may not work.")

    return True


if __name__ == "__main__":
    if env_check():
        mimir()
    else:
        sys.exit(1)
