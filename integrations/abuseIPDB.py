import requests
import os
from dotenv import load_dotenv
import re

ip_regex = re.compile(r"^(?:(?:25[0-5]|2[0-4]\d|1\d{2}|[1-9]?\d)\.){3}(?:25[0-5]|2[0-4]\d|1\d{2}|[1-9]?\d)$")

load_dotenv()
API_KEY = os.getenv("ABUSE_API_KEY")

def abuse_ip(ip_address):
    url = "https://api.abuseipdb.com/api/v2/check"
    headers = {
        "Key": API_KEY,
        "Accept": "application/json"
    }
    params = {
        "ipAddress": ip_address,
        "maxAgeInDays": 90,
        "verbose": True
    }

    response = requests.get(url, headers=headers, params=params)
    if response.status_code != 200:
        print(f"Error {response.status_code}: {response.text}")
        return

    data = response.json()["data"]

    print("=" * 60)
    print("ABUSEIPDB IP REPUTATION REPORT")
    print("=" * 60)
    print(f"IP Address : {data.get('ipAddress')}")
    print(f"Country    : {data.get('countryName')} ({data.get('countryCode')})")
    print(f"ISP        : {data.get('isp')}")
    print(f"Domain     : {data.get('domain') or 'N/A'}")
    print(f"Usage Type : {data.get('usageType')}")
    print("-" * 60)
    print(f"Abuse Score: {data.get('abuseConfidenceScore')} / 100")
    print(f"Reports    : {data.get('totalReports')} (from {data.get('numDistinctUsers')} distinct users)")
    print(f"Last Seen  : {data.get('lastReportedAt') or 'N/A'}")
    print("-" * 60)

    reports = data.get("reports", [])
    if reports:
        print("Recent Reports:")
        for r in reports[:3]:
            print(f"  • Date     : {r.get('reportedAt')}")
            print(f"    Reporter : {r.get('reporterCountryName')}")
            print(f"    Comment  : {r.get('comment') or 'No comment'}")
            print(f"    Categories: {', '.join(map(str, r.get('categories', [])))}")
            print("-" * 60)
    else:
        print("No recent reports available.")

    print("=" * 60, "\n")


if __name__ == "__main__":
    ip = input("Enter IP address: ").strip()
    abuse_ip(ip)