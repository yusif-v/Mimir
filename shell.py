import subprocess
import shlex
import os
import getpass
import readline
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

HISTORY_FILE = os.path.expanduser(".msh_history")

if os.path.exists(HISTORY_FILE):
    readline.read_history_file(HISTORY_FILE)

def dedup():
    seen = {}
    for i in range(readline.get_current_history_length(), 0, -1):
        cmd = readline.get_history_item(i)
        if cmd in seen:
            readline.remove_history_item(i-1)
        else:
            seen[cmd]=True


def mimir():
    print("Welcome to Mimir. Type 'help' for commands, 'exit' to quit.")
    while True:
        user = getpass.getuser()
        cwd = os.path.basename(os.getcwd()) or "/"
        prompt = f"\033[92m[{user}]\033[0m\033[96m[{cwd}]\033[0m|> "
        raw = input(prompt).strip().lower()
        if not raw:
            continue

        readline.add_history(raw)
        dedup()

        parts = shlex.split(raw)
        command, *args = parts

        if command == "exit":
            print("Exiting Mimir...")
            break

        elif command == "help":
            print("Available commands: help, exit, mhistory, hash, ipcheck")

        elif command == "mhistory":
            for i in range(1, readline.get_current_history_length() + 1):
                print(f"{i}: {readline.get_history_item(i)}")

        elif command == "hash":
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

        elif command == "ipcheck":
            if not args:
                print("Usage: ipcheck <ip address>")
                continue

            elif bool(abuseIPDB.ip_regex.match(args[0])):
                abuseIPDB.abuse_ip(args[0])
                continue

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

    readline.write_history_file(HISTORY_FILE)

if __name__ == "__main__":
    mimir()
