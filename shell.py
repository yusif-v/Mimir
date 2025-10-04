import subprocess
import shlex
import os
import getpass
from prompt_toolkit import PromptSession
from prompt_toolkit.history import FileHistory
from prompt_toolkit.formatted_text import ANSI
from integrations import malwareBazaar, abuseIPDB


def get_hash(args):
    result = subprocess.run(
        ["sha256sum", *args],
        capture_output=True,
        text=True,
        check=True,
    )
    print(result.stdout.strip().split()[0])
    malwareBazaar.mb_hash(result.stdout.strip().split()[0])


def parse_history(history_file):
    entries = []
    try:
        with open(history_file, "r") as f:
            for line in f:
                line = line.strip()
                if line and not line.startswith("#"):
                    cmd = line[1:].strip() if line.startswith("+") else line
                    entries.append(cmd)
    except FileNotFoundError:
        return []
    unique_entries = []
    seen = set()
    for cmd in reversed(entries):
        if cmd not in seen:
            unique_entries.append(cmd)
            seen.add(cmd)
    return unique_entries[-50:][::-1]


def display_history(history_file):
    entries = parse_history(history_file)
    if not entries:
        print("No history found.")
        return
    max_cmd_len = max(len(cmd) for cmd in entries)
    print("Num  Command" + " " * (max_cmd_len - 7))
    print("-" * (max_cmd_len + 6))
    for i, cmd in enumerate(entries, 1):
        print(f"{i:<4} {cmd:<{max_cmd_len}}")


def mimir():
    print("Welcome to Mimir. Type 'help' for commands, 'exit' to quit.")
    history_file = os.path.expanduser("~/.mhistory")
    user = getpass.getuser()
    cwd = os.path.basename(os.getcwd()) or "/"
    prompt = ANSI(f"\033[92m[{user}]\033[0m\033[96m[{cwd}]\033[0m|> ")
    session = PromptSession(
        message=prompt,
        history=FileHistory(history_file)
    )
    valid_commands = {"help", "exit", "hash", "ipcheck", "clear", "mhistory"}

    while True:
        try:
            raw = session.prompt().strip()
        except (EOFError, KeyboardInterrupt):
            print("\nExiting Mimir...")
            break
        if not raw:
            continue
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
            print("Available commands: help, exit, hash, ipcheck, clear, mhistory")
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
                get_hash(args)
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