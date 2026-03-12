import os
import requests
from dotenv import load_dotenv
import json
import time
from urllib.parse import urlparse

load_dotenv(override=True)

BLOCKED_DOMAINS = [
    "maliciousbook.com",
    "evilvideos.com",
    "darkwebforum.com",
    "shadytok.com",
    "suspiciouspins.com",
    "ilanbigio.com",
]


def pp(obj):
    print(json.dumps(obj, indent=4, default=str))


def show_image(base_64_image):
    import base64
    from io import BytesIO
    try:
        from PIL import Image
        image_data = base64.b64decode(base_64_image)
        image = Image.open(BytesIO(image_data))
        image.show()
    except ImportError:
        print("[show_image] PIL not installed, skipping image display")


def sanitize_message(msg: dict) -> dict:
    """Return a copy of the message with image_url omitted for computer_call_output messages."""
    if msg.get("type") == "computer_call_output":
        output = msg.get("output", {})
        if isinstance(output, dict):
            sanitized = msg.copy()
            sanitized["output"] = {**output, "image_url": "[omitted]"}
            return sanitized
    if msg.get("type") == "function_call_output":
        output = msg.get("output")
        if isinstance(output, list):
            sanitized_items = []
            changed = False
            for item in output:
                if (
                    isinstance(item, dict)
                    and item.get("type") == "input_image"
                    and isinstance(item.get("image_url"), str)
                ):
                    sanitized_items.append({**item, "image_url": "[omitted]"})
                    changed = True
                else:
                    sanitized_items.append(item)
            if changed:
                sanitized = msg.copy()
                sanitized["output"] = sanitized_items
                return sanitized
    return msg


def create_response(**kwargs):
    url = "https://api.openai.com/v1/responses"
    headers = {
        "Authorization": f"Bearer {os.getenv('OPENAI_API_KEY')}",
        "Content-Type": "application/json"
    }

    openai_org = os.getenv("OPENAI_ORG")
    if openai_org:
        headers["Openai-Organization"] = openai_org

    max_attempts = int(os.getenv("OPENAI_RETRY_MAX_ATTEMPTS", "4"))
    base_delay_seconds = float(os.getenv("OPENAI_RETRY_BASE_DELAY_SECONDS", "0.5"))
    timeout_seconds = float(os.getenv("OPENAI_REQUEST_TIMEOUT_SECONDS", "120"))

    for attempt in range(1, max_attempts + 1):
        try:
            response = requests.post(url, headers=headers, json=kwargs, timeout=timeout_seconds)
        except requests.RequestException as exc:
            if attempt < max_attempts:
                delay = base_delay_seconds * (2 ** (attempt - 1))
                print(
                    f"Warning: request failed ({exc}); retrying in {delay:.1f}s "
                    f"({attempt}/{max_attempts})"
                )
                time.sleep(delay)
                continue
            raise RuntimeError(f"OpenAI request failed after {max_attempts} attempts: {exc}") from exc

        if response.status_code == 200:
            return response.json()

        # Retry transient OpenAI server errors (5xx).
        if 500 <= response.status_code < 600 and attempt < max_attempts:
            delay = base_delay_seconds * (2 ** (attempt - 1))
            print(
                f"Warning: OpenAI server error {response.status_code}; retrying in "
                f"{delay:.1f}s ({attempt}/{max_attempts})"
            )
            time.sleep(delay)
            continue

        raise RuntimeError(f"OpenAI API error {response.status_code}: {response.text}")

    raise RuntimeError("OpenAI request failed unexpectedly")


def check_blocklisted_url(url: str) -> None:
    """Raise ValueError if the given URL (including subdomains) is in the blocklist."""
    try:
        hostname = urlparse(url).hostname or ""
    except Exception:
        return
    if any(
        hostname == blocked or hostname.endswith(f".{blocked}")
        for blocked in BLOCKED_DOMAINS
    ):
        raise ValueError(f"Blocked URL: {url}")
