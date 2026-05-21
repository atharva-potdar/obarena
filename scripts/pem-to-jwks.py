#!/usr/bin/env python3
"""Convert an RSA PEM public key to JWKS (JSON Web Key Set) format.

Envoy's jwt_authn filter requires JWKS JSON, not raw PEM.

Usage:
    python3 scripts/pem-to-jwks.py dev-jwt.pub > dev-jwt.jwks
"""

import base64
import json
import sys
from subprocess import check_output


def b64url(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).rstrip(b"=").decode()


def main():
    if len(sys.argv) != 2:
        print(f"Usage: {sys.argv[0]} <public_key.pem>", file=sys.stderr)
        sys.exit(1)

    pem_path = sys.argv[1]
    txt = check_output(
        ["openssl", "rsa", "-pubin", "-in", pem_path, "-text", "-noout"], text=True
    )

    mod_lines = []
    capture = False
    exp_val = None

    for line in txt.splitlines():
        if "Modulus:" in line:
            capture = True
            continue
        if "Exponent:" in line:
            capture = False
            raw = line.split("(")[1].split(")")[0]
            exp_val = int(raw, 16) if raw.startswith("0x") else int(raw)
            break
        if capture:
            mod_lines.append(line.strip().replace(":", ""))

    mod_bytes = bytes.fromhex("".join(mod_lines))
    # Strip leading zero byte (ASN.1 unsigned integer padding).
    if mod_bytes[0] == 0:
        mod_bytes = mod_bytes[1:]

    exp_bytes = exp_val.to_bytes((exp_val.bit_length() + 7) // 8, "big")

    jwks = {
        "keys": [
            {
                "kty": "RSA",
                "alg": "RS256",
                "use": "sig",
                "kid": "obarena-dev-1",
                "n": b64url(mod_bytes),
                "e": b64url(exp_bytes),
            }
        ]
    }

    print(json.dumps(jwks))


if __name__ == "__main__":
    main()
