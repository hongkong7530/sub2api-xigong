"""Operator-only cluster identity and enrollment tooling; never imports production secrets.

Run on the target Linux host. Private keys and the one-time token stay out of
command-line arguments, source control and stdout. CA signing is offline.
"""
from __future__ import annotations

import argparse
import getpass
import hashlib
import http.client
import json
import os
from pathlib import Path
import re
import ssl
import subprocess
import sys

IDENTITY = re.compile(r"^[a-z0-9][a-z0-9_-]{0,63}$")


def run(*args: str) -> bytes:
    result = subprocess.run(args, capture_output=True, check=False, timeout=30)
    if result.returncode:
        raise RuntimeError(f"{args[0]} failed (exit {result.returncode}); inspect protected local configuration")
    return result.stdout


def protected_directory(path: Path) -> Path:
    if not path.is_absolute() or path.is_symlink():
        raise ValueError("an absolute, non-symlink identity directory is required")
    path.mkdir(parents=True, exist_ok=True, mode=0o700)
    path.chmod(0o700)
    return path


def require_new(*paths: Path) -> None:
    if any(path.exists() for path in paths):
        raise ValueError("identity files already exist; refusing to replace them")


def create_ca(args: argparse.Namespace) -> None:
    directory = protected_directory(args.directory)
    key, cert = directory / "ca.key", directory / "ca.pem"
    require_new(key, cert)
    run("openssl", "req", "-x509", "-newkey", "ed25519", "-nodes", "-days", "3650",
        "-subj", "/CN=Sub2API Cluster CA", "-keyout", str(key), "-out", str(cert),
        "-addext", "basicConstraints=critical,CA:TRUE", "-addext", "keyUsage=critical,keyCertSign,cRLSign")
    key.chmod(0o600)
    print(json.dumps({"ca_certificate": str(cert), "private_key_exported": False}))


def create_request(args: argparse.Namespace) -> None:
    if not IDENTITY.fullmatch(args.node):
        raise ValueError("invalid node identity")
    directory = protected_directory(args.directory)
    key, csr = directory / "node.key", directory / "node.csr"
    require_new(key, csr)
    run("openssl", "req", "-new", "-newkey", "ed25519", "-nodes", "-keyout", str(key),
        "-out", str(csr), "-subj", "/CN=" + args.node,
        "-addext", "subjectAltName=DNS:" + args.node)
    key.chmod(0o600)
    print(json.dumps({"node_id": args.node, "certificate_request": str(csr), "private_key_exported": False}))


def sign_request(args: argparse.Namespace) -> None:
    if not IDENTITY.fullmatch(args.node):
        raise ValueError("invalid node identity")
    require_new(args.output)
    subject = run("openssl", "req", "-in", str(args.request), "-noout", "-subject", "-nameopt", "RFC2253").decode().strip()
    if subject != "subject=CN=" + args.node:
        raise ValueError("CSR identity does not match the approved node")
    extension = args.output.with_suffix(".extensions")
    require_new(extension)
    extension.write_text("basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=serverAuth,clientAuth\nsubjectAltName=DNS:" + args.node + "\n", encoding="ascii")
    try:
        run("openssl", "x509", "-req", "-in", str(args.request), "-CA", str(args.ca / "ca.pem"),
            "-CAkey", str(args.ca / "ca.key"), "-set_serial", "0x" + os.urandom(16).hex(),
            "-days", "90", "-extfile", str(extension), "-out", str(args.output))
    finally:
        extension.unlink(missing_ok=True)
    der = run("openssl", "x509", "-in", str(args.output), "-outform", "DER")
    print(json.dumps({"node_id": args.node, "certificate_sha256": hashlib.sha256(der).hexdigest(), "certificate": str(args.output)}))


def enroll(args: argparse.Namespace) -> None:
    from urllib.parse import urlsplit
    origin = urlsplit(args.controller)
    if origin.scheme != "https" or not origin.hostname or origin.username or origin.query or origin.fragment or origin.path not in ("", "/"):
        raise ValueError("controller must be an HTTPS origin")
    context = ssl.create_default_context(cafile=str(args.directory / "ca.pem"))
    context.minimum_version = ssl.TLSVersion.TLSv1_3
    context.load_cert_chain(str(args.directory / "node.pem"), str(args.directory / "node.key"))
    token = getpass.getpass("One-use join token: ")
    if not token or len(token) > 128:
        raise ValueError("invalid invitation")
    connection = http.client.HTTPSConnection(origin.hostname, origin.port or 443, context=context, timeout=10)
    try:
        connection.request("POST", "/cluster/enroll", body=json.dumps({"token": token}), headers={"Content-Type": "application/json"})
        response = connection.getresponse()
        body = response.read(16384)
        if response.status != 200:
            raise RuntimeError(f"enrollment rejected (HTTP {response.status}); invitation remains secret")
        result = json.loads(body)
        print(json.dumps({"node_id": result["node_id"], "state": result["state"], "revision": result["revision"], "traffic_enabled": False}))
    finally:
        connection.close()


def parser() -> argparse.ArgumentParser:
    root = argparse.ArgumentParser(description=__doc__)
    commands = root.add_subparsers(dest="action", required=True)
    ca = commands.add_parser("create-ca")
    ca.add_argument("--directory", required=True, type=Path)
    ca.set_defaults(run=create_ca)
    request = commands.add_parser("request-certificate")
    request.add_argument("--directory", required=True, type=Path)
    request.add_argument("--node", required=True)
    request.set_defaults(run=create_request)
    sign = commands.add_parser("sign-certificate")
    sign.add_argument("--ca", required=True, type=Path)
    sign.add_argument("--request", required=True, type=Path)
    sign.add_argument("--output", required=True, type=Path)
    sign.add_argument("--node", required=True)
    sign.set_defaults(run=sign_request)
    join = commands.add_parser("enroll")
    join.add_argument("--directory", required=True, type=Path)
    join.add_argument("--controller", required=True)
    join.set_defaults(run=enroll)
    return root


if __name__ == "__main__":
    try:
        args = parser().parse_args()
        args.run(args)
    except (ValueError, RuntimeError, OSError, subprocess.TimeoutExpired) as error:
        print(str(error), file=sys.stderr)
        raise SystemExit(1)
