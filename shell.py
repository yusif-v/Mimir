import subprocess


def mimir():
    print("Welcome to Mimir. Type 'help' for commands, 'exit' to quit.")
    while True:
        command = input("|> ").strip().lower()
        if command == "exit":
            print("Exiting Mimir...")
            break
        elif command == "help":
            print("Available commands: help, exit")
        elif command == "":
            continue
        else:
            try:
                result = subprocess.run(
                    command, capture_output=True, text=True, shell=True, check=True
                )
                print(result.stdout)
            except subprocess.CalledProcessError as e:
                print(f"Error: {e.stderr}")


if __name__ == "__main__":
    mimir()
