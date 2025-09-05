import requests
import os
import json
from dotenv import load_dotenv

load_dotenv()
API_KEY = os.getenv("OTX_API_KEY")

def otx_ip(ip_address, section="general"):
    url = f"https://otx.alienvault.com/api/v1/indicators/IPv4/{ip_address}/{section}"
    headers = {"X-OTX-API-KEY": API_KEY}

    response = requests.get(url, headers=headers)
    print(f"Status code: {response.status_code}")

    try:
        print(json.dumps(response.json(), indent=4))
    except Exception as e:
        print("Error parsing response:", e)
        print(response.text)

otx_ip("8.8.8.8", section="general")

