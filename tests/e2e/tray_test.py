import pyautogui
import time
import subprocess
import os
import sys
import psutil

ASSETS_DIR = os.path.join(os.path.dirname(__file__), "..", "..", "assets")
TRAY_ICON_IDLE = os.path.join(ASSETS_DIR, "tray-idle.png")
TRAY_ICON_CONNECTED = os.path.join(ASSETS_DIR, "tray-connected.png")
TRAY_ICON_ERROR = os.path.join(ASSETS_DIR, "tray-error.png")
REPORT_DIR = os.path.join(os.path.dirname(__file__), "reports")

os.makedirs(REPORT_DIR, exist_ok=True)


def wait_for_image(image_path, timeout=10, confidence=0.7):
    print(f"Waiting for image {os.path.basename(image_path)}...")
    start_time = time.time()
    while time.time() - start_time < timeout:
        try:
            location = pyautogui.locateCenterOnScreen(image_path, confidence=confidence)
            if location:
                print(f"Found image at {location}")
                return location
        except pyautogui.ImageNotFoundException:
            pass
        except Exception as e:
            print(f"Error during locate: {e}")
        time.sleep(1)

    print(f"Timeout waiting for image: {image_path}")
    return None


def take_screenshot(name):
    path = os.path.join(REPORT_DIR, f"{name}.png")
    screenshot = pyautogui.screenshot()
    screenshot.save(path)
    print(f"Saved screenshot: {path}")
    return path


def kill_process_tree(pid):
    try:
        parent = psutil.Process(pid)
        children = parent.children(recursive=True)
        for child in children:
            child.kill()
        parent.kill()
    except psutil.NoSuchProcess:
        pass


def run_test():
    print("Starting SerialHub tray test...")
    print("Starting SerialHub server process...")
    cmd = ["npx.cmd", "tsx", "src/server.ts", "-p", "COM7"]

    process = subprocess.Popen(
        cmd,
        cwd=os.path.join(os.path.dirname(__file__), "..", ".."),
        creationflags=subprocess.CREATE_NEW_PROCESS_GROUP,
    )

    try:
        print("Waiting 10 seconds for server and tray to initialize completely...")
        time.sleep(10)

        location = None
        current_state = ""

        for state, img_path in [
            ("error", TRAY_ICON_ERROR),
            ("idle", TRAY_ICON_IDLE),
            ("connected", TRAY_ICON_CONNECTED),
        ]:
            if os.path.exists(img_path):
                loc = wait_for_image(img_path, timeout=2, confidence=0.7)
                if loc:
                    location = loc
                    current_state = state
                    print(f"Found tray icon in '{state}' state!")
                    break
            else:
                print(f"Warning: Icon file not found: {img_path}")

        take_screenshot("tray_area_before_click")

        if not location:
            print("Failed to find tray icon on screen!")
            print(
                "Please ensure the system tray is visible (not hidden in the overflow menu) and the app started correctly."
            )
            for state, img_path in [
                ("error", TRAY_ICON_ERROR),
                ("idle", TRAY_ICON_IDLE),
                ("connected", TRAY_ICON_CONNECTED),
            ]:
                loc = wait_for_image(img_path, timeout=1, confidence=0.5)
                if loc:
                    print(f"Found something with very low confidence 0.5 for {state}")
            sys.exit(1)

        print(f"Right-clicking tray icon at {location}...")
        pyautogui.rightClick(location.x, location.y)

        time.sleep(2)

        take_screenshot("tray_menu_visible")

        report_path = os.path.join(REPORT_DIR, "test_report.html")
        with open(report_path, "w", encoding="utf-8") as f:
            f.write(f"""
            <html>
            <head><title>SerialHub Tray Test Report</title></head>
            <style>
                body {{ font-family: Arial, sans-serif; margin: 20px; }}
                .screenshot {{ max-width: 800px; border: 1px solid #ccc; margin-top: 10px; }}
                .success {{ color: green; font-weight: bold; }}
            </style>
            <body>
                <h1>SerialHub Tray Test Report</h1>
                <p>Status: <span class="success">PASSED</span></p>
                <p>Detected State: {current_state}</p>
                
                <h2>Tray Area Before Click</h2>
                <img class="screenshot" src="tray_area_before_click.png" />
                
                <h2>Tray Menu Visible</h2>
                <img class="screenshot" src="tray_menu_visible.png" />
            </body>
            </html>
            """)

        print(f"Test completed successfully! Report generated at: {report_path}")

    finally:
        print("Stopping SerialHub server process...")
        kill_process_tree(process.pid)


if __name__ == "__main__":
    print("Test will start in 2 seconds...")
    time.sleep(2)
    run_test()
