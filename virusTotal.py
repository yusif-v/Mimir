import requests
import json
import os
from dotenv import load_dotenv

load_dotenv()

API_KEY = os.getenv("VT_API_KEY")

def filehash (hash_file):
    url = f"https://www.virustotal.com/api/v3/files/{hash_file}"
    headers = {"x-apikey": API_KEY}

    response = requests.get(url, headers=headers)
    print(json.dumps(response.json(), indent=4))


if __name__ == "__main__" :
    file_hash = input("Enter file hash (MD5, SHA1, or SHA256): ").strip()
    filehash(file_hash)
