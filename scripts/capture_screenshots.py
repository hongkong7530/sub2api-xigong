"""Capture anonymous public pages without mocks, login or configuration writes."""
from __future__ import annotations

import argparse
import hashlib
import json
from datetime import datetime, timezone
from pathlib import Path
from urllib.parse import urlparse

from playwright.sync_api import sync_playwright


def capture(base_url: str, output: Path) -> list[dict]:
    parsed = urlparse(base_url)
    if parsed.scheme not in {"http", "https"} or not parsed.hostname or parsed.username or parsed.password:
        raise ValueError("Use an HTTP(S) base URL without credentials")
    output.mkdir(parents=True, exist_ok=True)
    results: list[dict] = []
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        try:
            for filename, path, width, height, section in [
                ("home-desktop.png", "/home", 1440, 1000, None),
                ("home-mobile.png", "/home", 393, 852, None),
                ("home-status.png", "/home", 1440, 1100, "#site-details"),
                ("model-plaza.png", "/model-plaza", 1440, 1000, None),
                ("partners.png", "/home?panel=partners", 1440, 1000, None),
            ]:
                context = browser.new_context(
                    viewport={"width": width, "height": height},
                    device_scale_factor=1,
                    locale="zh-CN",
                )
                try:
                    page = context.new_page()
                    errors: list[str] = []
                    page.on("pageerror", lambda error: errors.append(type(error).__name__))
                    response = page.goto(base_url.rstrip("/") + path, wait_until="networkidle", timeout=60000)
                    page.evaluate("async () => { await document.fonts.ready; await Promise.all(Array.from(document.images).map(i => i.decode().catch(() => {}))); }")
                    # The brand is drawn on canvas; font readiness alone precedes its assembly.
                    page.wait_for_timeout(3500)
                    pause = page.get_by_role("button", name="暂停轮播", exact=True)
                    if pause.count() and pause.first.is_visible():
                        pause.first.click()
                    if section:
                        page.locator(section).evaluate("element => window.scrollTo(0, element.getBoundingClientRect().top + window.scrollY - 120)")
                    target = output / filename
                    page.screenshot(path=str(target), full_page=False, animations="disabled")
                    results.append({
                        "file": filename,
                        "url": page.url,
                        "captured_at": datetime.now(timezone.utc).isoformat(),
                        "viewport": {"width": width, "height": height},
                        "http_status": response.status if response else None,
                        "title": page.title(),
                        "horizontal_overflow": page.evaluate("document.documentElement.scrollWidth > innerWidth"),
                        "page_errors": errors,
                        "sha256": hashlib.sha256(target.read_bytes()).hexdigest(),
                        "authentication": "anonymous",
                        "mocked_requests": False,
                    })
                finally:
                    context.close()
        finally:
            browser.close()
    (output / "capture-manifest.json").write_text(json.dumps(results, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    return results


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", default="https://xigong.cloud")
    parser.add_argument("--output", type=Path, default=Path("docs/screenshots"))
    args = parser.parse_args()
    print(json.dumps(capture(args.base_url, args.output), ensure_ascii=False, indent=2))
