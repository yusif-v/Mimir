import subprocess
import shlex
import os
import getpass
from prompt_toolkit import PromptSession
from prompt_toolkit.history import FileHistory
from prompt_toolkit.formatted_text import ANSI
from prompt_toolkit.completion import Completer, Completion
from integrations import malwareBazaar, abuseIPDB, urlHaus
from cli.history import display_history, save_history

path = os.path.expanduser(os.getenv("MIMIR_PATH"))

class MimirCompleter(Completer):
    def __init__(self, commands):
        self.commands = commands

    def get_completions(self, document, complete_event):
        text = document.text_before_cursor.lstrip()
        if " " not in text and not text.startswith(" "):
            for cmd in self.commands:
                if cmd.startswith(text):
                    yield Completion(cmd, start_position=-len(text))

def case_manager(case, action):
    if not path:
        print("Error: MIMIR_PATH is not set in your environment.")
        return None

    investigations_path = os.path.join(path, "Investigations")
    os.makedirs(investigations_path, exist_ok=True)
    case_path = os.path.join(investigations_path, case)

    actions = {
        "create": lambda: create_case(case_path, case),
        "open": lambda: open_case(case_path, case),
        "close": lambda: close_case(case_path, case)
    }

    action_func = actions.get(action)
    if action_func:
        return action_func()
    else:
        print(f"Unknown action: {action}")
        return None

def create_case(case_path, case):
    if os.path.exists(case_path):
        print(f"Case '{case}' already exists at {case_path}")
        return None
    else:
        os.makedirs(case_path)
        print(f"New case created: {case_path}")
        os.chdir(case_path)
        return case

def open_case(case_path, case):
    if os.path.exists(case_path):
        os.chdir(case_path)
        print(f"Opened case: {case_path}")
        return case
    else:
        print(f"Case '{case}' does not exist at {case_path}")
        return None

def close_case(case_path, case):
    print(f"{case} is closed")
    parent_dir = os.path.dirname(os.path.dirname(case_path))
    os.chdir(parent_dir)
    return None

def get_prompt(user, case=None):
    cwd = os.path.basename(os.getcwd()) or "/"
    if case:
        return ANSI(f"\033[92m[{user}]\033[0m\033[96m[mimir]\033[0m\033[93m[{case}]\033[0m|> ")
    return ANSI(f"\033[92m[{user}]\033[0m\033[96m[{cwd}]\033[0m|> ")

def mimir():
    print("Welcome to Mimir. Type 'help' for commands, 'exit' to quit.")
    history_file = os.path.expanduser(f"{path}/.mhistory")
    user = getpass.getuser()
    current_case = None
    commands = ['help', 'exit', 'hash', 'ipcheck', 'clear', 'mhistory', 'urlcheck', 'case']
    mimir_completer = MimirCompleter(commands)
    case_options = ["-n", "-o", "-c"]

    while True:
        prompt = get_prompt(user, current_case)
        session = PromptSession(message=prompt, history=FileHistory(history_file), completer=mimir_completer)
        try:
            raw = session.prompt().strip()
        except (EOFError, KeyboardInterrupt):
            print("\nExiting Mimir...")
            break
        if not raw:
            continue
        save_history(history_file, raw)
        try:
            parts = shlex.split(raw)
        except ValueError as e:
            print(f"Error parsing command: {e}")
            continue
        if not parts:
            continue
        command, *args = parts
        cmd = command.lower()
        if cmd == "exit":
            print("Exiting Mimir...")
            break
        elif cmd == "help":
            print("Available commands: " + ", ".join(commands))
        elif cmd == "clear":
            os.system("clear" if os.name != "nt" else "cls")
        elif cmd == "mhistory":
            display_history(history_file)
        elif cmd == "hash":
            if not args:
                print("Usage: hash <filename>, hash -h <hashstring>")
                continue
            elif "-h" in args:
                hashindex = args.index("-h")
                hashstring = args[hashindex + 1]
                malwareBazaar.mb_hash(hashstring)
                continue
            try:
                malwareBazaar.get_hash(args)
            except subprocess.CalledProcessError as e:
                print(f"Error: {e.stderr.strip()}")
        elif cmd == "ipcheck":
            if len(args) != 1:
                print("Usage: ipcheck <ip address>")
                continue
            ip = args[0]
            if abuseIPDB.ip_regex.match(ip):
                abuseIPDB.abuse_ip(ip)
            else:
                print("Invalid IP address")
            continue
        elif cmd == "urlcheck":
            if len(args) != 1:
                print("Usage: urlcheck <url>")
                continue
            url = args[0]
            urlHaus.urlcheck(url)
        elif cmd == "case":
            if len(args) < 2 or args[0] not in case_options:
                print(f"Usage: case [{' | '.join(case_options)}] \"case name\"")
                continue

            action = {"-n": "create", "-o": "open", "-c": "close"}.get(args[0])
            case_name = args[1].strip('"')

            new_case = case_manager(case_name, action)

            if action == "close":
                current_case = None
            elif new_case:
                current_case = new_case
        else:
            try:
                result = subprocess.run(
                    raw, capture_output=True, text=True, shell=True, check=True
                )
                print(result.stdout)
            except subprocess.CalledProcessError as e:
                print(f"Error: {e.stderr}")

if __name__ == "__main__":
    mimir()