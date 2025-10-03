import subprocess
import shlex
import os
import getpass
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

def mimir():
    print("Welcome to Mimir. Type 'help' for commands, 'exit' to quit.")
    while True:
        user = getpass.getuser()
        cwd = os.path.basename(os.getcwd()) or "/"
        prompt = f"\033[92m[{user}]\033[0m\033[96m[{cwd}]\033[0m|> "
        try:
            raw = input(prompt).strip()
        except (EOFError, KeyboardInterrupt):
            print("\nExiting Mimir...")
            break

        if not raw:
            continue

        # Split into command + args
        try:
            parts = shlex.split(raw)
        except ValueError as e:
            print(f"Error parsing command: {e}")
            continue

        command, *args = parts
        cmd = command.lower()

        if cmd == "exit":
            print("Exiting Mimir...")
            break

        elif cmd == "help":
            print("Available commands: help, exit, hash, ipcheck")

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