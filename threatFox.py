import requests
import json
import os
from dotenv import load_dotenv

load_dotenv()
API_KEY = os.getenv("MW_API_KEY")

def threatfox_lookup(hash_value):
    url = "https://threatfox-api.abuse.ch/api/v1/"
    headers = {"Auth-Key": API_KEY}
    data = {
        "query": "search_hash",
        "hash": hash_value
    }

    response = requests.post(url, headers=headers, data=data)
    print(json.dumps(response.json(), indent=4))

# Example usage
threatfox_lookup("72b7bdbd1362f833ed7a2e32f679a0fac64839aa98c515cbc5e97f6fcb6c32d8")
