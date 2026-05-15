#!/usr/bin/env python3
"""Simple XTerm UI Test"""

import time
import subprocess
import requests
from playwright.sync_api import sync_playwright

def main():
    # Start SerialHub
    print("Starting SerialHub...")
    proc = subprocess.Popen(["bin\\serialhub.exe", "--no-tray"], 
                           stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    time.sleep(5)
    
    try:
        # Wait for server
        server_ready = False
        for _ in range(20):
            try:
                if requests.get("http://127.0.0.1:5000/health", timeout=2).status_code == 200:
                    print("Server ready")
                    server_ready = True
                    break
            except Exception as e:
                print(f"Waiting for server... {e}")
                time.sleep(1)
        
        if not server_ready:
            print("Server failed to start")
            return
        
        # Run browser test
        with sync_playwright() as p:
            browser = p.chromium.launch(headless=False)
            page = browser.new_page(viewport={"width": 1200, "height": 800})
            
            print("Opening terminal...")
            page.goto("http://127.0.0.1:5000/terminal")
            time.sleep(2)
            
            # Take screenshot
            print("Taking screenshot...")
            page.screenshot(path="tests/e2e/xterm_test.png")
            print("Screenshot saved: tests/e2e/xterm_test.png")
            
            browser.close()
            print("Test complete!")
            
    finally:
        proc.terminate()
        proc.wait()

if __name__ == "__main__":
    main()
