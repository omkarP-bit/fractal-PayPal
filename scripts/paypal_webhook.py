#!/usr/bin/env python3
import base64
import json
import os
import sys
import urllib.error
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
EVENTS = [
    "CHECKOUT.ORDER.APPROVED",
    "CHECKOUT.ORDER.COMPLETED",
    "PAYMENT.CAPTURE.COMPLETED",
    "PAYMENT.CAPTURE.DENIED",
    "PAYMENT.CAPTURE.REFUNDED",
    "PAYMENT.CAPTURE.REVERSED",
]
DEFAULT_ACCOUNT = "00000000-0000-0000-0000-0000000000c2"


def load_env():
    env = {}
    path = ROOT / ".env"
    if path.exists():
        for line in path.read_text().splitlines():
            line = line.strip()
            if line and not line.startswith("#") and "=" in line:
                key, value = line.split("=", 1)
                env[key.strip()] = value.strip()
    env.update({k: v for k, v in os.environ.items() if k.startswith("PAYPAL_") or k in ("PUBLIC_TUNNEL_URL", "FRACTAL_PUBLIC_URL", "PROVIDER_ACCOUNT_ID")})
    return env


def api(env, method, path, token=None, payload=None):
    base = env.get("PAYPAL_BASE_URL", "https://api-m.sandbox.paypal.com").rstrip("/")
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    data = None
    if payload is not None:
        data = json.dumps(payload).encode()
    request = urllib.request.Request(base + path, data=data, headers=headers, method=method)
    return json.load(urllib.request.urlopen(request, timeout=30))


def access_token(env):
    client_id = env.get("PAYPAL_CLIENT_ID", "")
    secret = env.get("PAYPAL_CLIENT_SECRET", "")
    base = env.get("PAYPAL_BASE_URL", "https://api-m.sandbox.paypal.com").rstrip("/")
    if not client_id or not secret:
        sys.exit("PAYPAL_CLIENT_ID / PAYPAL_CLIENT_SECRET are not set in .env")
    auth = base64.b64encode(f"{client_id}:{secret}".encode()).decode()
    request = urllib.request.Request(
        base + "/v1/oauth2/token",
        data=b"grant_type=client_credentials",
        headers={"Authorization": "Basic " + auth, "Content-Type": "application/x-www-form-urlencoded"},
    )
    return json.load(urllib.request.urlopen(request, timeout=30))["access_token"]


def main():
    env = load_env()
    tunnel = (env.get("FRACTAL_PUBLIC_URL") or env.get("PUBLIC_TUNNEL_URL") or "").rstrip("/")
    if not tunnel:
        sys.exit("FRACTAL_PUBLIC_URL (or PUBLIC_TUNNEL_URL) is not set in .env")
    account = env.get("PROVIDER_ACCOUNT_ID", DEFAULT_ACCOUNT)
    url = f"{tunnel}/api/v1/webhooks/paypal/{account}"
    token = access_token(env)

    webhook_id = env.get("PAYPAL_WEBHOOK_ID", "")
    if webhook_id:
        try:
            api(env, "PATCH", f"/v1/notifications/webhooks/{webhook_id}", token, [{"op": "replace", "path": "/url", "value": url}])
            print(f"updated webhook {webhook_id} -> {url}")
            return
        except urllib.error.HTTPError as err:
            detail = err.read().decode()
            if err.code == 400 and "WEBHOOK_PATCH_REQUEST_NO_CHANGE" in detail:
                print(f"webhook {webhook_id} already points at {url}")
                return
            if err.code != 404:
                sys.exit(f"could not update webhook: {err.code} {detail[:300]}")
            print(f"webhook {webhook_id} no longer exists; creating a new one")

    created = api(env, "POST", "/v1/notifications/webhooks", token, {
        "url": url,
        "event_types": [{"name": name} for name in EVENTS],
    })
    print(f"created webhook {created['id']} -> {created['url']}")
    print(f"set PAYPAL_WEBHOOK_ID={created['id']} in .env and restart fractal-api")


if __name__ == "__main__":
    main()
