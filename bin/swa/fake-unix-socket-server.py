#!/usr/bin/env python3
"""Bind a Unix domain socket, accept connections, and close them immediately.

This creates the gRPC framing error used in scenario 4: the socket exists and
accepts TCP connections, but sends no valid gRPC preface — forcing the client to
report a framing or EOF error rather than ENOENT.

Usage:
    python3 bin/swa/fake-unix-socket-server.py <socket_path> [timeout_sec]

The server exits after <timeout_sec> seconds of inactivity (default: 8).
"""
import os
import socket
import sys


def main():
    if len(sys.argv) < 2:
        print(f"Usage: {sys.argv[0]} <socket_path> [timeout_sec]", file=sys.stderr)
        sys.exit(1)

    path = sys.argv[1]
    timeout = float(sys.argv[2]) if len(sys.argv) > 2 else 8.0

    sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    sock.bind(path)
    sock.listen(10)
    sock.settimeout(timeout)

    try:
        while True:
            try:
                conn, _ = sock.accept()
                conn.close()
            except socket.timeout:
                break
    finally:
        sock.close()
        try:
            os.unlink(path)  # noqa: S606 - developer CLI tool; socket path is caller-supplied by design
        except OSError:
            pass


if __name__ == "__main__":
    main()
