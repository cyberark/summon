#!/bin/sh
# Demo workload: reports the secrets it was started with, then idles until it
# is stopped. Summon stops it with SIGTERM when a secret rotates.
echo "app started (pid $$): DB_PASSWORD=${DB_PASSWORD:-<unset>} cert=$(cat tls/cert.pem 2>/dev/null || echo '<none>')"
trap 'echo "app stopping (pid $$)"; exit 0' TERM INT
while true; do
  sleep 1 &
  wait $!
done
