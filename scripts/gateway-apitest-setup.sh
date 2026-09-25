#!/bin/sh
set -eu

mkdir -p /fixtures /test-config
if [ -s /test-config/token ] && [ -s /test-config/limited-token ]; then
    exit 0
fi
printf '%s' '{"model":"sdk-test-text","embed_dimensions":8,"turns":[' > /fixtures/text.json
printf '%s' '{"model":"sdk-test-tools","turns":[' > /fixtures/tools.json
turn=0
while [ "$turn" -lt 32 ]; do
    if [ "$turn" -gt 0 ]; then
        printf ',' >> /fixtures/text.json
        printf ',' >> /fixtures/tools.json
    fi
    printf '%s' '{"text":"hello from gateway","usage":{"prompt_tokens":12,"completion_tokens":4}}' >> /fixtures/text.json
    printf '%s' '{"tool_calls":[{"id":"call_weather","name":"weather","arguments":{"city":"Berlin"}}],"usage":{"prompt_tokens":12,"completion_tokens":4}}' >> /fixtures/tools.json
    turn=$((turn + 1))
done
printf ']}' >> /fixtures/text.json
printf ']}' >> /fixtures/tools.json

contenox --data-dir /tmp/.contenox init
contenox --data-dir /tmp/.contenox backend add sdk-test-text --type scripted-test --script /fixtures/text.json
contenox --data-dir /tmp/.contenox backend add sdk-test-tools --type scripted-test --script /fixtures/tools.json
contenox --data-dir /tmp/.contenox gateway key create --client sdk-test-primary --models sdk-test-text,sdk-test-tools --output-allowance 1000 --input-allowance 10000 --authority-private-key-file /run/secrets/authority > /test-config/token
contenox --data-dir /tmp/.contenox gateway key create --client sdk-test-limited --models sdk-test-text --output-allowance 4 --input-allowance 1000 --authority-private-key-file /run/secrets/authority > /test-config/limited-token

attempt=0
until [ "$attempt" -ge 30 ]; do
    if curl -fsS -H "Authorization: Bearer $(cat /test-config/token)" http://gateway:11435/api/tags | grep -q 'sdk-test-text'; then
        exit 0
    fi
    attempt=$((attempt + 1))
    sleep 1
done
echo 'gateway did not discover the scripted test models' >&2
exit 1
