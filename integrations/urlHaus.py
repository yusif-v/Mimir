import requests
import os
from dotenv import load_dotenv

load_dotenv()
API_KEY = os.getenv("ACH_API_KEY")

def urlcheck(input_url):
    url = "https://urlhaus-api.abuse.ch/v1/url/"
    headers = {"Auth-Key": API_KEY} if API_KEY else {}
    data = {"url": input_url}

    try:
        response = requests.post(url, headers=headers, data=data, timeout=15)
        response.raise_for_status()
    except requests.RequestException as e:
        print(f"[-] Network error: {e}")
        return None

    try:
        json_resp = response.json()
    except ValueError:
        print("[-] Invalid JSON response from URLhaus")
        return None

    if response.status_code == 401 or json_resp.get("error") == "Unauthorized":
        print("[-] Unauthorized: URLhaus API requires a valid Auth-Key (set ABUSE_API_KEY in .env).")
        return json_resp

    status = json_resp.get("query_status")
    if status == "ok":
        print(f"[+] URL found on URLhaus!")
        print(f"    URL: {json_resp.get('url') or input_url}")
        print(f"    Threat: {json_resp.get('threat')}")
        print(f"    Status: {json_resp.get('url_status')}")
        print(f"    First seen: {json_resp.get('date_added')}")
        print(f"    Reporter: {json_resp.get('reporter')}")
        tags = json_resp.get("tags") or []
        print(f"    Tags: {', '.join(tags) if tags else 'None'}")

        payloads = json_resp.get("payloads") or []
        if payloads:
            print("\nAssociated Payloads:")
            for p in payloads[:5]:
                fname = p.get("file_name") or p.get("payload_filename")
                ftype = p.get("file_type") or p.get("payload_type")
                sha256 = p.get("sha256_hash") or p.get("payload_sha256")
                print(f" - {fname} ({ftype}) | SHA256: {sha256}")

    elif status == "no_results":
        print("[-] URL not found in URLhaus database.")
    else:
        print(f"[!] Query failed or unexpected status: {status}")
        print(json_resp)

    return json_resp

if __name__ == "__main__" :
    url_input = input("Enter the url: ")
    print(url_input)
    urlcheck(url_input)