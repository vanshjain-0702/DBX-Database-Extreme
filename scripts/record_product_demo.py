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
from pathlib import Path

import websocket

DEBUG = "http://127.0.0.1:9222"
SITE = "http://127.0.0.1:8765"
DASH = "http://127.0.0.1:8000"
TENANT_ID = "demo-walk"
TENANT_NAME = "Demo Walk"


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


T0 = 0.0


def mark(name: str) -> None:
    line = f"{time.time() - T0:.2f} {name}"
    print(f"MARK {line}", flush=True)
    p = Path("/tmp/dbx-demo/marks.txt")
    p.parent.mkdir(parents=True, exist_ok=True)
    with p.open("a") as fh:
        fh.write(line + "\n")


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


def type_text(text: str, delay_ms: int = 48) -> None:
    # xdotool type interprets some glyphs; keep to ascii for the demo.
    xdotool("type", "--delay", str(delay_ms), "--", text)


def key(*keys: str) -> None:
    xdotool("key", "--", *keys)


def js_click_expr(cdp: CDP, expr: str, timeout: float = 10.0) -> None:
    deadline = time.time() + timeout
    while time.time() < deadline:
        ok = cdp.eval(f"""
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
""")
        if ok:
            time.sleep(0.25)
            return
        time.sleep(0.2)
    raise TimeoutError(f"js click missing {expr}")


def js_click_sel(cdp: CDP, selector: str, timeout: float = 10.0) -> None:
    js_click_expr(cdp, f"document.querySelector({json.dumps(selector)})", timeout)


def js_focus(cdp: CDP, selector: str) -> None:
    ok = cdp.eval(f"""
(() => {{
  const el = document.querySelector({json.dumps(selector)});
  if (!el) return false;
  el.scrollIntoView({{block: "center"}});
  el.focus();
  el.click();
  return true;
}})()
""")
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


def scroll_page(cdp: CDP, pixels: int, steps: int = 8) -> None:
    step = pixels / steps
    for _ in range(steps):
        cdp.eval(f"window.scrollBy(0, {step})")
        time.sleep(0.42)


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


def seed_bench_vectors(cdp: CDP, tenant: str) -> None:
    """VADD 128-d docs so Vector Playground search returns real hits."""
    expr = f"""
(async () => {{
  const token = localStorage.getItem('dbx_token');
  const q = async (command) => {{
    const res = await fetch({json.dumps('/t/' + tenant + '/query')}, {{
      method: 'POST',
      headers: {{
        'Content-Type': 'application/json',
        Authorization: 'Bearer ' + token
      }},
      body: JSON.stringify({{ command }})
    }});
    return await res.json();
  }};
  const vec = (seed) => Array.from({{ length: 128 }}, (_, i) =>
    (Math.sin((seed + 1) * 12.9898 + i * 0.37)).toFixed(5)
  );
  const docs = [
    ['invoice-q3', 'Acme unpaid invoice Q3 enterprise billing and collections'],
    ['legal-memo', 'Legal memo on tenant isolation and data residency'],
    ['harbor-session', 'Harbor tenant session token refresh runbook']
  ];
  for (const [id, text] of docs) {{
    await q(['VADD', 'bench_vectors', id, ...vec(id.length)]);
    await q(['SET', 'doc:bench_vectors:' + id, JSON.stringify({{ page_content: text }})]);
  }}
  return 'seeded';
}})()
"""
    print("seed vectors:", cdp.eval(expr, await_promise=True))


def console_cmd(cdp: CDP, cmd: str) -> None:
    js_focus(cdp, '.console-term input, form input[placeholder="PING"]')
    pause(0.15)
    key("ctrl+a")
    type_text(cmd, delay_ms=42)
    key("Return")
    pause(2.4)


def run_site(cdp: CDP) -> None:
    mark("site_home")
    cdp.goto(SITE + "/", settle=2.6)
    pause(4.5)
    scroll_page(cdp, 80, steps=3)
    pause(3.0)

    # Isolation bench
    mark("bench")
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
        pause(6.0)
        click_sel(cdp, "[data-pause]")
    except Exception as exc:
        print("bench failed:", exc)

    mark("site_why")
    scroll_page(cdp, 700, steps=8)
    pause(3.5)
    scroll_page(cdp, 700, steps=8)
    pause(3.0)

    # Command palette → features
    try:
        click_sel(cdp, "[data-cmdk-open]")
        pause(0.6)
        type_text("features", delay_ms=40)
        pause(1.0)
        key("Return")
        pause(3.2)
    except Exception:
        cdp.goto(SITE + "/features.html")

    mark("site_pages")
    for path, scroll in [
        ("/features.html", 1400),
        ("/architecture.html", 900),
        ("/start.html", 700),
        ("/performance.html", 800),
        ("/docs/quickstart.html", 600),
        ("/docs/api.html", 700),
        ("/security.html", 700),
    ]:
        cdp.goto(SITE + path, settle=2.0)
        if path == "/start.html":
            mark("start")
        elif path == "/performance.html":
            mark("perf")
        elif path == "/docs/api.html":
            mark("docs")
        elif path == "/security.html":
            mark("security")
        pause(2.2)
        if path == "/start.html":
            try:
                click_js(cdp, find_btn("From source"))
                pause(2.2)
                click_js(cdp, find_btn("Compose"))
                pause(2.2)
                click_js(cdp, find_btn("Docker"))
                pause(1.8)
            except Exception as exc:
                print("start tabs:", exc)
        scroll_page(cdp, scroll, steps=max(5, scroll // 180))
        pause(2.6)


def run_dashboard(cdp: CDP) -> None:
    tid = TENANT_ID
    mark("dash_login")
    cdp.goto(DASH + "/login", settle=2.6)
    pause(2.5)
    key("Escape")
    pause(0.4)
    # React controlled inputs: set via the native value setter, then click Sign In.
    # xdotool-only typing loses to Chrome autofill and often hits the username field.
    cdp.eval("""
(() => {
  const set = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set;
  const user = document.querySelector('#login-username');
  const pass = document.querySelector('#login-password');
  if (user) {
    set.call(user, 'admin');
    user.dispatchEvent(new Event('input', { bubbles: true }));
  }
  if (pass) {
    pass.focus();
    set.call(pass, 'adminadminadmin');
    pass.dispatchEvent(new Event('input', { bubbles: true }));
  }
})()
""")
    pause(0.8)
    js_click_sel(cdp, "#login-submit-btn")
    try:
        cdp.wait_js(
            "document.body.innerText.includes('Tenants') || document.body.innerText.includes('Provision')",
            timeout=18,
        )
    except TimeoutError:
        print("login wait failed; retry via /api/login")
        cdp.eval(
            """
(async () => {
  const res = await fetch('/api/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: 'admin', password: 'adminadminadmin' })
  });
  const data = await res.json();
  if (!data.token) throw new Error('no token');
  localStorage.setItem('dbx_token', data.token);
  location.href = '/';
})()
""",
            await_promise=True,
        )
        cdp.wait_js(
            "document.body.innerText.includes('Tenants') || document.body.innerText.includes('Provision')",
            timeout=18,
        )
    pause(2.0)

    already = False
    try:
        already = bool(
            cdp.eval(
                f"!![...document.querySelectorAll('h3,button')].find(e => "
                f"(e.textContent||'').includes({json.dumps(TENANT_NAME)}))"
            )
        )
    except Exception:
        already = False

    if not already:
        try:
            click_js(cdp, find_btn("Provision"))
            pause(0.8)
            click_js(cdp, "document.querySelector('.modal-content input.input-field')")
            js_focus(cdp, ".modal-content input.input-field")
            type_text(TENANT_NAME, delay_ms=30)
            pause(0.3)
            cdp.eval(
                "document.querySelectorAll('.modal-content input.input-field')[1].focus()"
            )
            pause(0.15)
            type_text(tid, delay_ms=30)
            pause(0.4)
            click_js(cdp, "document.querySelector('.modal-content .btn-primary')")
            pause(3.0)
        except Exception as exc:
            print("provision:", exc)

    try:
        click_js(
            cdp,
            "([...document.querySelectorAll('button,h3')].find(e => "
            f"(e.textContent||'').includes({json.dumps(TENANT_NAME)}))||null)",
        )
        pause(3.5)
    except Exception as exc:
        print("open tenant:", exc)
        cdp.goto(DASH + f"/cluster/{tid}/overview", settle=2.0)

    mark("dash_overview")
    pause(2.4)
    try:
        click_js(cdp, find_btn("Backup"))
        pause(0.8)
        key("Return")
        pause(2.8)
    except Exception as exc:
        print("backup:", exc)

    mark("dash_keys")
    cdp.goto(DASH + f"/cluster/{tid}/keys", settle=2.0)
    pause(2.4)
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

    mark("dash_console")
    cdp.goto(DASH + f"/cluster/{tid}/terminal", settle=2.0)
    pause(2.4)
    for cmd in [
        "PING",
        "SET session:42 onboarding",
        "GET session:42",
        "KEYS *",
        "VADD memories doc:1 0.1 0.2 0.9",
        "VSEARCH memories 0.1 0.2 0.8 5",
    ]:
        try:
            console_cmd(cdp, cmd)
        except Exception as exc:
            print("console", cmd, exc)
    pause(2.0)

    mark("dash_explorer")
    cdp.goto(DASH + f"/cluster/{tid}/explorer", settle=2.2)
    pause(3.2)
    try:
        js_focus(cdp, 'input[placeholder="Filter keys…"]')
        type_text("session", delay_ms=40)
        pause(1.4)
        click_js(
            cdp,
            "([...document.querySelectorAll('button,div,span')].find(e => "
            "(e.textContent||'').includes('session:42'))||null)",
        )
        pause(2.2)
        key("ctrl+a")
        key("BackSpace")
        pause(0.6)
    except Exception as exc:
        print("explorer filter:", exc)
    try:
        click_js(cdp, find_btn("New key"))
        pause(0.7)
        click_js(
            cdp,
            "document.querySelector('.modal-content input, .modal-content .input-field')",
        )
        type_text("scratch:1", delay_ms=28)
        key("Tab")
        type_text("hello-from-explorer", delay_ms=24)
        pause(0.3)
        click_js(cdp, find_btn("Save key"))
        pause(1.6)
        click_js(cdp, find_btn("Refresh"))
        pause(1.6)
    except Exception as exc:
        print("explorer new key:", exc)
        try:
            key("Escape")
        except Exception:
            pass

    try:
        seed_bench_vectors(cdp, tid)
    except Exception as exc:
        print("seed vectors:", exc)

    mark("dash_vector")
    cdp.goto(DASH + f"/cluster/{tid}/vector", settle=2.2)
    pause(2.0)
    try:
        cdp.eval("""
(() => {
  const sel = document.querySelector('select.input-field');
  if (!sel) return;
  sel.value = 'bench_vectors';
  sel.dispatchEvent(new Event('change', { bubbles: true }));
})()
""")
        pause(1.2)
        js_focus(cdp, "textarea.input-field")
        type_text("enterprise billing invoice", delay_ms=38)
        pause(0.6)
        js_focus(cdp, 'input[placeholder="e.g. enterprise"]')
        type_text("enterprise", delay_ms=36)
        pause(0.5)
        click_js(cdp, find_btn("Run search"))
        try:
            cdp.wait_js(
                "document.querySelector('.result-card') || "
                "document.body.innerText.includes('No matches') || "
                "document.body.innerText.includes('Failed')",
                timeout=18,
            )
        except TimeoutError:
            print("vector search still loading")
        pause(5.5)
    except Exception as exc:
        print("vector playground:", exc)
        pause(4.0)

    mark("dash_palette")
    try:
        key("ctrl+k")
        pause(1.0)
        type_text("hardware", delay_ms=40)
        pause(0.8)
        key("Return")
        pause(3.0)
    except Exception as exc:
        print("palette:", exc)
        cdp.goto(DASH + f"/cluster/{tid}/hardware", settle=1.8)
        pause(2.4)

    mark("runtime")
    for path in [
        f"/cluster/{tid}/storage",
        f"/cluster/{tid}/network",
        f"/cluster/{tid}/hosting",
    ]:
        cdp.goto(DASH + path, settle=1.8)
        pause(3.2)
        if path.endswith("/hosting"):
            try:
                click_js(cdp, find_btn("Refresh"))
                pause(2.0)
            except Exception:
                pass

    mark("dash_settings")
    cdp.goto(DASH + "/settings", settle=1.8)
    pause(2.4)
    try:
        click_js(cdp, find_exact("Security"))
        pause(2.6)
        click_js(cdp, find_exact("API keys"))
        pause(1.2)
        click_js(cdp, find_btn("Generate"))
        pause(0.7)
        js_focus(cdp, ".modal-content input.input-field, .modal-content input")
        type_text("ci-bot", delay_ms=32)
        pause(0.3)
        click_js(cdp, "document.querySelector('.modal-content .btn-primary')")
        pause(2.4)
        click_js(cdp, find_btn("I have copied the key"))
        pause(1.2)
        click_js(cdp, find_exact("Replication"))
        pause(3.2)
    except Exception as exc:
        print("settings:", exc)
        for path in ["/settings/security", "/settings/keys", "/settings/replication"]:
            cdp.goto(DASH + path, settle=1.6)
            pause(2.4)

    mark("close")
    cdp.goto(SITE + "/demo.html", settle=2.2)
    pause(6.0)


def main() -> None:
    import sys

    global T0
    T0 = time.time()
    Path("/tmp/dbx-demo/marks.txt").write_text("")
    mark("start")
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
