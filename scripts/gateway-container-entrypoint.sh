#!/bin/sh
set -eu

cp /run/secrets/authority /tmp/gateway-authority
cp /run/secrets/token_key /tmp/gateway-token-key
chown contenox:contenox /tmp/gateway-authority /tmp/gateway-token-key
chmod 0400 /tmp/gateway-authority /tmp/gateway-token-key
exec su-exec contenox /usr/local/bin/contenox "$@"
