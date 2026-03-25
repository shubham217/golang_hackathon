#!/bin/bash

# --- 1. Preparation ---
echo "🚀 Initializing Steady Stream Gateway..."
go mod tidy
go build -o gateway .

# --- 2. Unit Testing ---
echo "🧪 Running Unit Tests..."
if go test -v ./...; then
    echo -e "\033[0;32m✅ Tests Passed!\033[0m"
else
    echo -e "\033[0;31m❌ Tests Failed! Aborting.\033[0m"
    exit 1
fi

# --- 3. Start Server ---
echo "🌐 Starting Gateway on :8080..."
./gateway &
GATEWAY_PID=$!

# Wait for server to be ready
sleep 2

# --- 4. Rate Limit Test ---
URL="http://localhost:8080/api/fast"
echo "🔥 Performing Rate Limit Test (15 requests)..."

for i in {1..15}; do
    # Perform curl and capture headers/status
    RESPONSE=$(curl -s -i $URL)
    
    # Extract data
    STATUS=$(echo "$RESPONSE" | grep "HTTP/" | awk '{print $2}')
    REMAINING=$(echo "$RESPONSE" | grep "X-RateLimit-Remaining" | awk '{print $2}' | tr -d '\r')
    RETRY_AFTER=$(echo "$RESPONSE" | grep "Retry-After" | awk '{print $2}' | tr -d '\r')

    if [ "$STATUS" == "200" ]; then
        echo -e "\033[0;32m[Req $i] Status: $STATUS | Remaining: $REMAINING\033[0m"
    elif [ "$STATUS" == "429" ]; then
        echo -e "\033[0;31m[Req $i] Status: $STATUS | LIMIT HIT! Retry-After: ${RETRY_AFTER}s\033[0m"
    else
        echo -e "[Req $i] Status: $STATUS"
    fi
done

# --- 5. Cleanup ---
echo "🛑 Shutting down Gateway..."
kill $GATEWAY_PID
rm gateway
echo "✨ Done!"
