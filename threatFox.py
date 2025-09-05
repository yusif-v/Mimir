import requests
import json
import os
from dotenv import load_dotenv

load_dotenv()
API_KEY = os.getenv("ACH_API_KEY")

def tf_hash(file_hash):
    url = "https://threatfox-api.abuse.ch/api/v1/"
    headers = {"Auth-Key": API_KEY}
    data = {
        "query": "search_hash",
        "hash": file_hash
    }

    response = requests.post(url, headers=headers, json=data)
    print(json.dumps(response.json(), indent=4))


tf_hash("72b7bdbd1362f833ed7a2e32f679a0fac64839aa98c515cbc5e97f6fcb6c32d8")

if __name__ == "__main__" :
    hash_value = input("Enter file hash (MD5, SHA1, or SHA256): ").strip()
    tf_hash(hash_value)