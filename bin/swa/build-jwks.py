#!/usr/bin/env python3
"""Build a Conjur-format JWKS object from an RSA PEM public key.

Usage:
    python3 bin/swa/build-jwks.py <rsa_public_key.pem>

Output (stdout):
    {"keys":[{"kty":"RSA","kid":"swa-e2e-key-1","alg":"RS256","use":"sig","n":"...","e":"..."}]}

Uses only Python standard library.
"""
import base64
import json
import os
import sys


def b64url(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).rstrip(b"=").decode()


def read_tlv(data: bytes, pos: int):
    tag = data[pos]; pos += 1
    first = data[pos]; pos += 1
    if first < 0x80:
        length = first
    elif first == 0x81:
        length = data[pos]; pos += 1
    elif first == 0x82:
        length = (data[pos] << 8) | data[pos + 1]; pos += 2
    else:
        raise ValueError(f"Unsupported ASN.1 length encoding: 0x{first:02x}")
    return tag, data[pos:pos + length], pos + length


def rsa_pub_to_jwk(pem_file: str) -> dict:
    pem = open(os.path.realpath(pem_file)).read()  # noqa: SIM115 - developer CLI tool; path is caller-supplied by design
    b64_body = "".join(
        line for line in pem.splitlines() if not line.startswith("-----")
    )
    der = base64.b64decode(b64_body)

    _, outer, _ = read_tlv(der, 0)           # SEQUENCE (SubjectPublicKeyInfo)
    _, _, pos = read_tlv(outer, 0)            # SEQUENCE (AlgorithmIdentifier) — skip
    _, bit_str, _ = read_tlv(outer, pos)      # BIT STRING
    inner = bit_str[1:]                       # drop the unused-bits byte
    _, rsa_seq, _ = read_tlv(inner, 0)        # SEQUENCE (RSAPublicKey)
    _, n_bytes, p2 = read_tlv(rsa_seq, 0)     # INTEGER (modulus n)
    _, e_bytes, _ = read_tlv(rsa_seq, p2)     # INTEGER (public exponent e)

    return {
        "kty": "RSA", "kid": "swa-e2e-key-1",
        "alg": "RS256", "use": "sig",
        "n": b64url(n_bytes.lstrip(b"\x00")),
        "e": b64url(e_bytes.lstrip(b"\x00")),
    }


def main():
    if len(sys.argv) != 2:
        print(f"Usage: {sys.argv[0]} <rsa_public_key.pem>", file=sys.stderr)
        sys.exit(1)

    jwk = rsa_pub_to_jwk(sys.argv[1])
    print(json.dumps({"keys": [jwk]}))


if __name__ == "__main__":
    main()
