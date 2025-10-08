import os
import subprocess
import venv
from dotenv import load_dotenv
import warnings

warnings.filterwarnings("ignore", category=UserWarning, module="urllib3")

HOME_DIR = os.path.expanduser("~")
PROJECT_DIR = os.path.join(HOME_DIR, "Mimir")
SUBDIRS = ["Investigations", "Reports"]
API_KEYS = ["OTX_API_KEY", "ABUSE_API_KEY", "ACH_API_KEY", "VT_API_KEY"]
VENV_DIR = os.path.join(PROJECT_DIR, ".venv")
REQUIREMENTS_FILE = os.path.join(os.path.dirname(__file__), "requirements.txt")
FLAG_FILE = os.path.join(PROJECT_DIR, ".deps_installed")

def structure():
    message = []
    if not os.path.exists(PROJECT_DIR):
        os.makedirs(PROJECT_DIR)
        message.append(f"[setup] Created main folder: {PROJECT_DIR}")
    for sub in SUBDIRS:
        path = os.path.join(PROJECT_DIR, sub)
        if not os.path.exists(path):
            os.makedirs(path)
            message.append(f"[setup] Created subfolder: {sub}")
    env_path = os.path.join(PROJECT_DIR, ".env")
    if not os.path.exists(env_path):
        with open(env_path, "w") as f:
            f.write("# Mimir Environment Variables\n")
            for key in API_KEYS:
                f.write(f"{key}=\n")
        message.append(f"[setup] Created .env file at {env_path}")
    return message

def setup_venv():
    message = []
    if not os.path.exists(VENV_DIR):
        venv.create(VENV_DIR, with_pip=True)
        message.append(f"[setup] Created virtual environment at {VENV_DIR}")
    if os.path.exists(FLAG_FILE):
        return message
    pip_executable = os.path.join(VENV_DIR, "bin", "pip") if os.name != "nt" else os.path.join(VENV_DIR, "Scripts", "pip.exe")
    if os.path.exists(REQUIREMENTS_FILE):
        try:
            subprocess.check_call([pip_executable, "install", "-r", REQUIREMENTS_FILE], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            with open(FLAG_FILE, "w") as f:
                f.write("Dependencies installed")
            message.append("[setup] Installed dependencies from requirements.txt.")
        except subprocess.CalledProcessError:
            message.append("[setup] Failed to install dependencies.")
    else:
        message.append("[setup] requirements.txt not found; skipped dependency installation.")
    return message

def env_check():
    env_path = os.path.join(PROJECT_DIR, ".env")
    load_dotenv(env_path)
    missing = [k for k in API_KEYS if not os.getenv(k)]
    if missing:
        return [f"[setup] Missing API keys: {', '.join(missing)}"]
    return []

def setup():
    message = []
    message.extend(structure())
    message.extend(setup_venv())
    message.extend(env_check())
    if message:
        for msg in message:
            print(msg)
    return not ("Missing API keys" in " ".join(message))

if __name__ == "__main__":
    success = setup()
    if messages := (structure() + setup_venv() + env_check()):
        print("\nMimir setup completed, but some API keys may be missing." if not success else "\nMimir setup completed successfully.")
    print("Welcome to Mimir. Type 'help' for commands, 'exit' to quit.")