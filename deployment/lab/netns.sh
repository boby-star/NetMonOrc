#!/bin/sh
set -eu
if [ "$1" = target ]; then
    ip address add 198.18.0.80/32 dev lo
    exec python3 /fixture.py
fi
ip route replace 198.18.0.80/32 via "$NMO_GATEWAY"
touch /run/netns-ready
exec sleep infinity
