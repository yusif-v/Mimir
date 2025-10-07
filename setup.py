import os
import subprocess
import venv
from dotenv import load_dotenv

HOME_DIR = os.path.expanduser("~")
PROJECT_DIR = os.path.join(HOME_DIR, "Mimir")
SUBDIRS = ["Investigations", "Reports"]
API_KEYS = ["OTX_API_KEY", "ABUSE_API_KEY", "ACH_API_KEY", "VT_API_KEY"]
VENV_DIR = os.path.join(PROJECT_DIR, ".venv")
REQUIREMENTS_FILE = os.path.join(os.path.dirname(__file__), "requirements.txt")


def create_structure():
    messages = []

    if not os.path.exists(PROJECT_DIR):
        os.makedirs(PROJECT_DIR)
        messages.append(f"[setup] Created main folder: {PROJECT_DIR}")

    for sub in SUBDIRS:
        path = os.path.join(PROJECT_DIR, sub)
        if not os.path.exists(path):
            os.makedirs(path)
            messages.append(f"[setup] Created subfolder: {sub}")

    env_path = os.path.join(PROJECT_DIR, ".env")
    if not os.path.exists(env_path):
        with open(env_path, "w") as f:
            f.write("# Mimir Environment Variables\n")
            for key in API_KEYS:
                f.write(f"{key}=\n")
        messages.append(f"[setup] Created .env file at {env_path}")

    return messages


def setup_virtualenv():
    messages = []

    if not os.path.exists(VENV_DIR):
        venv.create(VENV_DIR, with_pip=True)
        messages.append(f"[setup] Created virtual environment at {VENV_DIR}")
    else:
        messages.append("[setup] Virtual environment already exists.")

    pip_executable = os.path.join(VENV_DIR, "bin", "pip") if os.name != "nt" else os.path.join(VENV_DIR, "Scripts", "pip.exe")

    if os.path.exists(REQUIREMENTS_FILE):
        try:
            subprocess.check_call([pip_executable, "install", "-r", REQUIREMENTS_FILE])
            messages.append("[setup] Installed dependencies from requirements.txt.")
        except subprocess.CalledProcessError:
            messages.append("[setup] Failed to install dependencies.")
    else:
        messages.append("[setup] requirements.txt not found; skipped dependency installation.")

    return messages


def env_check():
    env_path = os.path.join(PROJECT_DIR, ".env")
    load_dotenv(env_path)

    missing = [k for k in API_KEYS if not os.getenv(k)]
    if missing:
        return [f"[setup] Missing API keys: {', '.join(missing)}"]
    return ["[setup] All API keys are configured."]


def setup():
    messages = []
    messages.extend(create_structure())
    messages.extend(setup_virtualenv())
    messages.extend(env_check())

    for msg in messages:
        print(msg)

    return not ("No API keys configured" in " ".join(messages))


if __name__ == "__main__":
    success = setup()
    if success:
        print("\nMimir setup completed successfully.")
    else:
        print("\nMimir setup completed, but some API keys are missing.")