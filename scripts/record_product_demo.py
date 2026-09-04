#!/usr/bin/env python3
"""Drive a headed Chrome window through the DBX site + dashboard for a screen recording.

Requires: google-chrome-stable on DISPLAY, remote debugging on 9222,
marketing site on :8765, orchestrator on :8000, python3-websocket.

  make site &
  make run-dev &
  google-chrome-stable --remote-debugging-port=9222 --user-data-dir=/tmp/dbx-demo-chrome \
    --no-first-run --disable-features=PasswordManager \
    --start-maximized http://127.0.0.1:8765/
  python3 scripts/record_product_demo.py
"""
from __future__ import annotations

import json
import subprocess
import time
import urllib.request

import websocket

DEBUG = "http://127.0.0.1:9222"
SITE = "http://127.0.0.1:8765"
DASH = "http://127.0.0.1:8000"


class CDP:
    def __init__(self, url: str) -> None:
        self.ws = websocket.create_connection(url, suppress_origin=True)
        self.n = 0
        self._call("Page.enable")
        self._call("Runtime.enable")

    def _call(self, method: str, **params):
        self.n += 1
        mid = self.n
        self.ws.send(json.dumps({"id": mid, "method": method, "params": params}))
        while True:
            msg = json.loads(self.ws.recv())
            if msg.get("id") == mid:
                if "error" in msg:
                    raise RuntimeError(f"{method}: {msg['error']}")
                return msg.get("result") or {}

    def eval(self, expression: str, await_promise: bool = False):
        result = self._call(
            "Runtime.evaluate",
            expression=expression,
            returnByValue=True,
            awaitPromise=await_promise,
        )
        if result.get("exceptionDetails"):
            raise RuntimeError(result["exceptionDetails"])
        value = (result.get("result") or {}).get("value")
        return value

    def goto(self, url: str, settle: float = 1.4) -> None:
        self._call("Page.navigate", url=url)
        time.sleep(settle)
        for _ in range(40):
            ready = self.eval("document.readyState")
            if ready == "complete":
                break
            time.sleep(0.15)
        time.sleep(0.4)

    def wait_js(self, expression: str, timeout: float = 12.0):
        deadline = time.time() + timeout
        last = None
        while time.time() < deadline:
            try:
                last = self.eval(expression)
            except RuntimeError:
                last = None
            if last:
                return last
            time.sleep(0.2)
        raise TimeoutError(f"wait_js: {expression!r} last={last!r}")

    def screen_point(self, selector: str | None = None, js_el: str | None = None):
        if selector:
            finder = f"document.querySelector({json.dumps(selector)})"
        else:
            finder = js_el or "null"
        expr = f"""
(() => {{
  const el = {finder};
  if (!el) return null;
  el.scrollIntoView({{block: "center", inline: "nearest"}});
  const r = el.getBoundingClientRect();
  const chromeH = Math.max(0, window.outerHeight - window.innerHeight);
  const chromeW = Math.max(0, window.outerWidth - window.innerWidth);
  return {{
    x: Math.round(window.screenX + chromeW / 2 + r.left + r.width / 2),
    y: Math.round(window.screenY + chromeH + r.top + r.height / 2),
    w: r.width,
    h: r.height,
    ok: r.width > 0 && r.height > 0
  }};
}})()
"""
        return self.eval(expr)

    def close(self) -> None:
        try:
            self.ws.close()
        except Exception:
            pass


def xdotool(*args: str) -> None:
    subprocess.check_call(["xdotool", *args], stdout=subprocess.DEVNULL)


def move_click(x: int, y: int, double: bool = False) -> None:
    xdotool("mousemove", "--sync", str(x), str(y))
    time.sleep(0.12)
    if double:
        xdotool("click", "--repeat", "2", "--delay", "80", "1")
    else:
        xdotool("click", "1")
    time.sleep(0.18)


def type_text(text: str, delay_ms: int = 28) -> None:
    # xdotool type interprets some glyphs; keep to ascii for the demo.
    xdotool("type", "--delay", str(delay_ms), "--", text)


def key(*keys: str) -> None:
    xdotool("key", "--", *keys)


def js_click_expr(cdp: CDP, expr: str, timeout: float = 10.0) -> None:
    deadline = time.time() + timeout
    while time.time() < deadline:
        ok = cdp.eval(
            f"""
(() => {{
  const el = {expr};
  if (!el) return false;
  el.scrollIntoView({{block: "center", inline: "nearest"}});
  el.dispatchEvent(new MouseEvent("pointerdown", {{bubbles:true}}));
  el.dispatchEvent(new MouseEvent("mousedown", {{bubbles:true}}));
  el.dispatchEvent(new MouseEvent("mouseup", {{bubbles:true}}));
  el.click();
  return true;
}})()
"""
        )
        if ok:
            time.sleep(0.25)
            return
        time.sleep(0.2)
    raise TimeoutError(f"js click missing {expr}")


def js_click_sel(cdp: CDP, selector: str, timeout: float = 10.0) -> None:
    js_click_expr(cdp, f"document.querySelector({json.dumps(selector)})", timeout)


def js_focus(cdp: CDP, selector: str) -> None:
    ok = cdp.eval(
        f"""
(() => {{
  const el = document.querySelector({json.dumps(selector)});
  if (!el) return false;
  el.scrollIntoView({{block: "center"}});
  el.focus();
  el.click();
  return true;
}})()
"""
    )
    if not ok:
        raise TimeoutError(f"focus {selector}")
    time.sleep(0.2)


def click_sel(cdp: CDP, selector: str, timeout: float = 10.0) -> None:
    try:
        deadline = time.time() + min(timeout, 3.0)
        box = None
        while time.time() < deadline:
            box = cdp.screen_point(selector=selector)
            if box and box.get("ok"):
                break
            time.sleep(0.15)
        if box and box.get("ok"):
            move_click(int(box["x"]), int(box["y"]))
            return
    except Exception:
        pass
    js_click_sel(cdp, selector, timeout=timeout)


def click_js(cdp: CDP, js_el: str, timeout: float = 10.0) -> None:
    try:
        deadline = time.time() + min(timeout, 3.0)
        box = None
        while time.time() < deadline:
            box = cdp.screen_point(js_el=js_el)
            if box and box.get("ok"):
                break
            time.sleep(0.15)
        if box and box.get("ok"):
            move_click(int(box["x"]), int(box["y"]))
            return
    except Exception:
        pass
    js_click_expr(cdp, js_el, timeout=timeout)


def scroll_page(cdp: CDP, pixels: int, steps: int = 6) -> None:
    step = pixels / steps
    for _ in range(steps):
        cdp.eval(f"window.scrollBy(0, {step})")
        time.sleep(0.28)


def pause(seconds: float) -> None:
    time.sleep(seconds)


def connect() -> CDP:
    with urllib.request.urlopen(DEBUG + "/json/list", timeout=5) as resp:
        tabs = json.load(resp)
    page = next((t for t in tabs if t.get("type") == "page"), tabs[0])
    return CDP(page["webSocketDebuggerUrl"])


def bench_send(cdp: CDP, command: str) -> None:
    click_sel(cdp, 'form[data-term-form] input[name="cmd"]')
    # select all + type
    key("ctrl+a")
    type_text(command, delay_ms=22)
    pause(0.3)
    click_sel(cdp, 'form[data-term-form] button[type="submit"]')
    pause(1.1)


def find_btn(text: str) -> str:
    return (
        "([...document.querySelectorAll('button,a')].find(e => "
        f"(e.textContent||'').replace(/\\s+/g,' ').trim().includes({json.dumps(text)}))||null)"
    )


def find_exact(text: str) -> str:
    return (
        "([...document.querySelectorAll('button,a')].find(e => "
        f"(e.textContent||'').replace(/\\s+/g,' ').trim() === {json.dumps(text)})||null)"
    )


def run_site(cdp: CDP) -> None:
    cdp.goto(SITE + "/", settle=2.0)
    pause(2.5)
    scroll_page(cdp, 120, steps=3)
    pause(2.0)

    # Isolation bench
    try:
        click_sel(cdp, "[data-pause]")
        pause(0.6)
        click_sel(cdp, "[data-replay]")
        pause(1.2)
        click_sel(cdp, '[data-pick="harbor"]')
        pause(0.8)
        bench_send(cdp, "AUTH harbor")
        bench_send(cdp, "SET session:1 harbor-intake")
        click_sel(cdp, '[data-pick="acme"]')
        pause(0.7)
        bench_send(cdp, "GET")
        bench_send(cdp, "VADD memories doc:1 [0.12, 0.81, 0.44]")
        click_sel(cdp, '[data-cabinet="harbor"]')
        pause(2.2)
        key("Escape")
        pause(0.4)
        click_sel(cdp, "[data-replay]")
        pause(4.5)
        click_sel(cdp, "[data-pause]")
    except Exception as exc:
        print("bench failed:", exc)

    scroll_page(cdp, 900, steps=8)
    pause(2.0)
    scroll_page(cdp, 900, steps=8)
    pause(1.6)

    # Command palette → features
    try:
        click_sel(cdp, "[data-cmdk-open]")
        pause(0.6)
        type_text("features", delay_ms=40)
        pause(0.7)
        key("Return")
        pause(2.0)
    except Exception:
        cdp.goto(SITE + "/features.html")

    for path, scroll in [
        ("/features.html", 1400),
        ("/architecture.html", 1100),
        ("/start.html", 700),
        ("/performance.html", 900),
        ("/docs/index.html", 400),
        ("/docs/quickstart.html", 700),
        ("/docs/api.html", 800),
        ("/pricing.html", 500),
        ("/security.html", 700),
        ("/contact.html", 500),
        ("/demo.html", 500),
    ]:
        cdp.goto(SITE + path)
        pause(1.4)
        if path == "/start.html":
            try:
                click_js(cdp, find_btn("From source"))
                pause(1.6)
                click_js(cdp, find_btn("Compose"))
                pause(1.6)
                click_js(cdp, find_btn("Docker"))
                pause(1.2)
            except Exception as exc:
                print("start tabs:", exc)
        scroll_page(cdp, scroll, steps=max(4, scroll // 180))
        pause(1.5)


def run_dashboard(cdp: CDP) -> None:
    cdp.goto(DASH + "/login", settle=2.2)
    pause(1.5)
    js_focus(cdp, "#login-password")
    type_text("adminadminadmin", delay_ms=32)
    pause(0.4)
    js_click_sel(cdp, "#login-submit-btn")
    cdp.wait_js(
        "document.body.innerText.includes('Tenants') || document.body.innerText.includes('Provision')",
        timeout=20,
    )
    pause(2.0)

    try:
        click_js(cdp, find_btn("Provision"))
        pause(0.8)
        # Name then tenant id
        click_js(cdp, "document.querySelector('.modal-content input.input-field')")
        js_focus(cdp, ".modal-content input.input-field")
        type_text("Demo Acme", delay_ms=30)
        pause(0.3)
        cdp.eval("document.querySelectorAll('.modal-content input.input-field')[1].focus()")
        pause(0.15)
        type_text("demo-acme", delay_ms=30)
        pause(0.4)
        click_js(cdp, "document.querySelector('.modal-content .btn-primary')")
        pause(2.5)
    except Exception as exc:
        print("provision:", exc)

    # Open tenant card
    try:
        click_js(
            cdp,
            "([...document.querySelectorAll('button,h3')].find(e => "
            "(e.textContent||'').includes('Demo Acme'))||null)",
        )
        pause(2.5)
    except Exception as exc:
        print("open tenant:", exc)
        cdp.goto(DASH + "/cluster/demo-acme/overview", settle=2.0)

    pause(2.0)
    try:
        click_js(cdp, find_btn("Backup"))
        pause(0.6)
        # confirm() is native; xdotool Return
        key("Return")
        pause(1.8)
    except Exception as exc:
        print("backup:", exc)

    # Tenant keys
    cdp.goto(DASH + "/cluster/demo-acme/keys", settle=1.8)
    pause(1.5)
    try:
        click_js(cdp, find_btn("Mint key"))
        pause(0.8)
        click_js(cdp, find_exact("Mint"))
        pause(2.8)
        click_js(cdp, find_btn("I have copied the secret"))
        pause(0.8)
    except Exception as exc:
        print("mint key:", exc)
        key("Escape")

    # Console
    cdp.goto(DASH + "/cluster/demo-acme/terminal", settle=1.8)
    pause(1.4)
    for cmd in [
        "PING",
        "SET session:42 onboarding",
        "GET session:42",
        "VADD memories doc:1 0.1 0.2 0.9",
        "VSEARCH memories 0.1 0.2 0.8 5",
    ]:
        try:
            js_focus(cdp, '.console-term input, form input[placeholder="PING"]')
            type_text(cmd, delay_ms=24)
            key("Return")
            pause(1.3)
        except Exception as exc:
            print("console", cmd, exc)

    # Explorer
    cdp.goto(DASH + "/cluster/demo-acme/explorer", settle=2.0)
    pause(2.0)
    try:
        click_js(
            cdp,
            "([...document.querySelectorAll('button,div,span')].find(e => "
            "(e.textContent||'').includes('session:42'))||null)",
        )
        pause(1.8)
    except Exception:
        pass
    try:
        click_js(cdp, find_btn("New key"))
        pause(0.7)
        click_js(cdp, "document.querySelector('.modal-content input, .modal-content .input-field')")
        type_text("scratch:1", delay_ms=28)
        key("Tab")
        type_text("hello-from-explorer", delay_ms=24)
        pause(0.3)
        click_js(cdp, find_btn("Save key"))
        pause(1.4)
    except Exception as exc:
        print("explorer new key:", exc)
        try:
            key("Escape")
        except Exception:
            pass

    cdp.goto(DASH + "/cluster/demo-acme/vector", settle=1.6)
    pause(3.0)

    for path in [
        "/cluster/demo-acme/hardware",
        "/cluster/demo-acme/storage",
        "/cluster/demo-acme/network",
        "/cluster/demo-acme/hosting",
        "/settings",
        "/settings/security",
        "/settings/replication",
        "/",
    ]:
        cdp.goto(DASH + path, settle=1.3)
        pause(1.8)

    cdp.goto(SITE + "/demo.html", settle=1.8)
    pause(3.5)


def main() -> None:
    import sys

    cdp = connect()
    try:
        if len(sys.argv) > 1 and sys.argv[1] == "dashboard":
            run_dashboard(cdp)
        else:
            run_site(cdp)
            run_dashboard(cdp)
        print("DEMO_SCRIPT_OK")
    finally:
        cdp.close()


if __name__ == "__main__":
    main()
