#!/bin/bash

URL="http://localhost:8080/api/fast"
echo "🚀 Starting Rate Limit Test (15 requests to /api/fast)..."

for i in {1..15}; do
    # Perform curl and capture headers/status
    RESPONSE=$(curl -s -i $URL)
    
    # Extract data using grep/awk
    STATUS=$(echo "$RESPONSE" | grep "HTTP/" | awk '{print $2}')
    REMAINING=$(echo "$RESPONSE" | grep "X-RateLimit-Remaining" | awk '{print $2}' | tr -d '\r')
    RETRY_AFTER=$(echo "$RESPONSE" | grep "Retry-After" | awk '{print $2}' | tr -d '\r')

    if [ "$STATUS" == "200" ]; then
        echo -e "\033[0;32m[Req $i] IP: 127.0.0.1 | Status: $STATUS | Remaining: $REMAINING\033[0m"
    else
        echo -e "\033[0;31m[Req $i] IP: 127.0.0.1 | Status: $STATUS | LIMIT HIT! Retry-After: ${RETRY_AFTER}s\033[0m"
    fi
done
