import koffi from "koffi";

export const isWindows = process.platform === "win32";

let consoleHidden = false;

const SW_HIDE = 0;
const SW_SHOW = 5;

let kernel32: ReturnType<typeof koffi.load> | null = null;
let user32: ReturnType<typeof koffi.load> | null = null;
let getConsoleWindow: (() => bigint) | null = null;
let showWindow: ((hwnd: bigint, cmd: number) => number) | null = null;

function initFFI(): void {
  if (!isWindows || kernel32) return;

  try {
    kernel32 = koffi.load("kernel32.dll");
    user32 = koffi.load("user32.dll");

    getConsoleWindow = kernel32.func("intptr GetConsoleWindow()");
    showWindow = user32.func("int ShowWindow(intptr, int)");
  } catch {
    // FFI 初始化失败
  }
}

export function hideConsole(): void {
  if (!isWindows || consoleHidden) return;

  initFFI();

  if (getConsoleWindow && showWindow) {
    const hwnd = getConsoleWindow();
    if (hwnd !== 0n) {
      showWindow(hwnd, SW_HIDE);
      consoleHidden = true;
    }
  }
}

export function showConsole(): void {
  if (!isWindows || !consoleHidden) return;

  initFFI();

  if (getConsoleWindow && showWindow) {
    const hwnd = getConsoleWindow();
    if (hwnd !== 0n) {
      showWindow(hwnd, SW_SHOW);
      consoleHidden = false;
    }
  }
}

export function toggleConsole(): boolean {
  if (consoleHidden) {
    showConsole();
  } else {
    hideConsole();
  }
  return !consoleHidden;
}

export function isConsoleHidden(): boolean {
  return consoleHidden;
}