#!/usr/bin/env python3
# find-signing-ca.py — Identify the CA cert that signed the current SVID.
#
# Usage:
#   python3 find-signing-ca.py <bundle.pem> <svid.pem> <swa-server-addr>
#
# Arguments:
#   bundle.pem      Trust bundle from the SWA Agent (may contain multiple certs)
#   svid.pem        Current X.509 SVID (leaf cert, possibly with chain)
#   swa-server-addr SWA Server address for TLS chain fallback (host:port)
#
# Outputs the CA cert PEM to stdout. When the signing CA is an intermediate,
# outputs the intermediate concatenated with its root so Conjur can verify
# an SVID regardless of whether the TLS client sends only the leaf or the
# full leaf+intermediate chain. Diagnostic messages go to stderr.
#
# Search strategy (stops at first match):
#   1. Trust bundle certs — subject or verify match against the SVID issuer
#   2. Intermediate certs in the SVID chain file (when chain len > 1)
#   3. Intermediate certs from the SWA Server TLS chain

import subprocess  # nosec B404 — openssl invocations use list form; all inputs validated
import re
import tempfile
import os
import sys


def _validate_inputs(bundle_path, svid_path, server_addr):
    """Validate all user-supplied inputs before any subprocess call."""
    for path, name in [(bundle_path, 'bundle.pem'), (svid_path, 'svid.pem')]:
        if not os.path.isfile(path):
            print(f"ERROR: {name} is not a readable file: {path!r}", file=sys.stderr)
            sys.exit(1)
    # server_addr must be host:port (IPv4/IPv6/hostname) — no shell metacharacters.
    if not re.match(r'^[A-Za-z0-9._\[\]:-]+$', server_addr):
        print(f"ERROR: invalid server address format: {server_addr!r}", file=sys.stderr)
        sys.exit(1)


def cert_subject_norm(pem_text):
    """Return lowercased, space-normalised subject DN for a single PEM cert."""
    with tempfile.NamedTemporaryFile(mode='w', suffix='.pem', delete=False) as cf:
        cf.write(pem_text + '\n')
        certfile = cf.name
    try:
        out = subprocess.run(  # nosec B603 B607 — list form; certfile is a controlled temp file
            ['openssl', 'x509', '-in', certfile, '-noout', '-subject'],
            capture_output=True, text=True).stdout.strip()
    finally:
        os.unlink(certfile)
    return re.sub(r'\s+', ' ', out.replace('subject=', '').strip()).lower()


def get_issuer_norm(pem_text):
    """Return lowercased, space-normalised issuer DN for a single PEM cert."""
    with tempfile.NamedTemporaryFile(mode='w', suffix='.pem', delete=False) as cf:
        cf.write(pem_text + '\n')
        certfile = cf.name
    try:
        out = subprocess.run(  # nosec B603 B607 — list form; certfile is a controlled temp file
            ['openssl', 'x509', '-in', certfile, '-noout', '-issuer'],
            capture_output=True, text=True).stdout.strip()
    finally:
        os.unlink(certfile)
    return re.sub(r'\s+', ' ', out.replace('issuer=', '').strip()).lower()


def verify_signs(ca_pem, leaf_path):
    """Return True if ca_pem verifies the leaf cert at leaf_path."""
    with tempfile.NamedTemporaryFile(mode='w', suffix='.pem', delete=False) as cf:
        cf.write(ca_pem + '\n')
        cafile = cf.name
    try:
        result = subprocess.run(  # nosec B603 B607 — list form; cafile is a temp file, leaf_path is validated
            ['openssl', 'verify', '-partial_chain', '-CAfile', cafile, leaf_path],
            capture_output=True, text=True)
    finally:
        os.unlink(cafile)
    return 'OK' in result.stdout


def walk_to_root(cert_pem, all_certs, depth=0):
    """Walk up the issuer chain to the self-signed root.

    Uses all_certs as the issuer lookup pool. Returns the root cert PEM when
    found, or cert_pem when already a root or the issuer is not in the pool.
    """
    if depth > 10:
        return cert_pem
    subj = cert_subject_norm(cert_pem)
    iss = get_issuer_norm(cert_pem)
    if subj == iss:
        return cert_pem
    for c in all_certs:
        if cert_subject_norm(c) == iss:
            print(f"  root walk: {subj!r} issued by {iss!r}", file=sys.stderr, flush=True)
            return walk_to_root(c, all_certs, depth + 1)
    print(f"  root walk: issuer {iss!r} not in bundle; using {subj!r}", file=sys.stderr, flush=True)
    return cert_pem


def find_signing_ca(certs, svid_issuer_norm, svid_path, source):
    """Return the first cert in certs whose subject or verify output matches."""
    for i, cert in enumerate(certs):
        subj = cert_subject_norm(cert)
        print(f"  {source}[{i}] subject: {subj}", file=sys.stderr, flush=True)
        if subj == svid_issuer_norm:
            print(f"  Signing CA (DN match from {source}): {subj}", file=sys.stderr, flush=True)
            return cert
        if verify_signs(cert, svid_path):
            print(f"  Signing CA (verify match from {source}): {subj}", file=sys.stderr, flush=True)
            return cert
    return None


def main():
    if len(sys.argv) != 4:
        print("Usage: find-signing-ca.py <bundle.pem> <svid.pem> <swa-server-addr>",
              file=sys.stderr)
        sys.exit(1)
    bundle_path = os.path.realpath(sys.argv[1])
    svid_path = os.path.realpath(sys.argv[2])
    server_addr = sys.argv[3]
    _validate_inputs(bundle_path, svid_path, server_addr)

    svid_issuer = subprocess.run(  # nosec B603 B607 — list form; svid_path validated by _validate_inputs
        ['openssl', 'x509', '-in', svid_path, '-noout', '-issuer'],
        capture_output=True, text=True).stdout.strip()
    svid_issuer_norm = re.sub(r'\s+', ' ', svid_issuer.replace('issuer=', '').strip()).lower()
    print(f"  SVID issuer: {svid_issuer_norm}", file=sys.stderr, flush=True)

    with open(bundle_path) as f:
        bundle = f.read()
    bundle_certs = re.findall(
        r'-----BEGIN CERTIFICATE-----.*?-----END CERTIFICATE-----', bundle, re.DOTALL)
    print(f"  Bundle has {len(bundle_certs)} cert(s).", file=sys.stderr, flush=True)

    ca = find_signing_ca(bundle_certs, svid_issuer_norm, svid_path, 'bundle')

    if ca is None:
        with open(svid_path) as f:
            svid_chain = f.read()
        chain_certs = re.findall(
            r'-----BEGIN CERTIFICATE-----.*?-----END CERTIFICATE-----', svid_chain, re.DOTALL)
        print(f"  SVID chain has {len(chain_certs)} cert(s).", file=sys.stderr, flush=True)
        if len(chain_certs) > 1:
            ca = find_signing_ca(chain_certs[1:], svid_issuer_norm, svid_path, 'svid-chain')

    if ca is None:
        print(f"  Trying SWA Server TLS chain at {server_addr}...", file=sys.stderr, flush=True)
        result = subprocess.run(  # nosec B603 B607 — list form; server_addr validated by _validate_inputs
            ['openssl', 's_client', '-connect', server_addr, '-showcerts'],
            input='', capture_output=True, text=True, timeout=15)
        server_certs = re.findall(
            r'-----BEGIN CERTIFICATE-----.*?-----END CERTIFICATE-----',
            result.stdout, re.DOTALL)
        print(f"  SWA Server TLS chain has {len(server_certs)} cert(s).", file=sys.stderr, flush=True)
        if len(server_certs) > 1:
            ca = find_signing_ca(server_certs[1:], svid_issuer_norm, svid_path, 'server-chain')

    if ca is None:
        print("  WARNING: signing CA not found in bundle, SVID chain, or TLS chain.",
              file=sys.stderr)
        sys.exit(1)

    root_ca = walk_to_root(ca, bundle_certs)
    if root_ca is not ca:
        print(f"  Using intermediate+root CA chain: "
              f"{cert_subject_norm(ca)!r} + {cert_subject_norm(root_ca)!r}",
              file=sys.stderr, flush=True)
        print(ca.strip() + '\n' + root_ca.strip())
    else:
        print(root_ca)


if __name__ == '__main__':
    main()
