import os
import shutil
from dotenv import load_dotenv

HOME_DIR = os.path.expanduser("~")
PROJECT_DIR = os.path.join(HOME_DIR, "Mimir")
ENV_FILE = os.path.join(PROJECT_DIR, ".env")
ENV_SAMPLE_FILE = os.path.join(os.path.dirname(__file__), "env.sample")  # sample stays with code
HISTORY_FILE = os.path.join(PROJECT_DIR, ".m_history")
SUBDIRS = ["Investigations", "Reports"]

API_KEYS = ["OTX_API_KEY", "ABUSE_API_KEY", "ACH_API_KEY", "VT_API_KEY"]

def create_structure():
    """Ensure required directories and files exist under ~/Mimir."""
    messages = []

    # Base project dir
    os.makedirs(PROJECT_DIR, exist_ok=True)

    # Subdirs
    for subdir in SUBDIRS:
        path = os.path.join(PROJECT_DIR, subdir)
        if not os.path.exists(path):
            os.makedirs(path)
            messages.append(f"[setup] Created directory: {path}")

    # .env file
    if not os.path.exists(ENV_FILE):
        if os.path.exists(ENV_SAMPLE_FILE):
            shutil.copy(ENV_SAMPLE_FILE, ENV_FILE)
            messages.append("[setup] No .env found. Created from env.sample.")
        else:
            with open(ENV_FILE, "w") as f:
                f.write("# Add your API keys here\n")
                for key in API_KEYS:
                    f.write(f"{key}=\n")
            messages.append("[setup] No .env found. Created an empty one.")

    # .m_history
    if not os.path.exists(HISTORY_FILE):
        with open(HISTORY_FILE, "w") as f:
            f.write("# Mimir history file\n")
        messages.append("[setup] .m_history created.")

    return messages


def env_check():
    """Load and check API keys."""
    load_dotenv(ENV_FILE)

    missing = []
    for key in API_KEYS:
        value = os.getenv(key)
        if not value:
            missing.append(key)

    messages = []
    if not missing:
        return messages  # silent if all good
    elif len(missing) == len(API_KEYS):
        messages.append("[setup] No API keys configured. Add at least one in .env.")
    else:
        messages.append(f"[setup] Some keys missing: {', '.join(missing)}")

    return messages


def setup():
    """Run structure + env check. Print only if changes/missing."""
    messages = []
    messages.extend(create_structure())
    messages.extend(env_check())

    for msg in messages:
        print(msg)

    # Return True only if at least one key exists
    return not ("No API keys configured" in " ".join(messages))