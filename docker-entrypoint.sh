#!/bin/sh
set -e

if [ -z "$TELEGRAM_BOT_TOKEN" ]; then
    echo "❌ Error: TELEGRAM_BOT_TOKEN not set"
    exit 1
fi

if [ -z "$TELEGRAM_API_ID" ]; then
    echo "❌ Error: TELEGRAM_API_ID not set"
    exit 1
fi

if [ -z "$TELEGRAM_API_HASH" ]; then
    echo "❌ Error: TELEGRAM_API_HASH not set"
    exit 1
fi

if [ -z "$OWNER_ID" ]; then
    echo "❌ Error: OWNER_ID not set"
    exit 1
fi

mkdir -p /app/data /app/logs

echo "✓ Starting ApexClaw..."
echo "  - Bot Token: ${TELEGRAM_BOT_TOKEN:0:10}***"
echo "  - API ID: $TELEGRAM_API_ID"
echo "  - Owner ID: $OWNER_ID"
echo "  - Max Iterations: ${MAX_ITERATIONS:-10}"

exec /app/apexclaw
