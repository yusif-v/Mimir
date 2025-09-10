import subprocess
import shlex
from integrations import malwareBazaar

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
        raw = input("|> ").strip().lower()
        if not raw:
            continue

        parts = shlex.split(raw)
        command, *args = parts

        if command == "exit":
            print("Exiting Mimir...")
            break
        elif command == "help":
            print("Available commands: help, exit")
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
