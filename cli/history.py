import os

def parse_history(history_file):
    try:
        with open(history_file, "r") as f:
            entries = [line.strip() for line in f if line.strip() and not line.startswith(("#", "+"))]
        return list(dict.fromkeys(entries[-50:]))[::-1]
    except FileNotFoundError:
        return []

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

def save_history(history_file, command):
    os.makedirs(os.path.dirname(history_file), exist_ok=True)
    with open(history_file, "a") as f:
        f.write(command + "\n")