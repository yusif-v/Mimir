import requests
import os
from dotenv import load_dotenv

load_dotenv()
API_KEY=os.getenv("WB_API_KEY")

def report():
    print(API_KEY)

if __name__ == "__main__":
    report()