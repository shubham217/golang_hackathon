$url = "http://localhost:8080/api/fast"
Write-Host "🚀 Starting Rate Limit Test (15 requests to /api/fast)..." -ForegroundColor Cyan

for ($i = 1; $i -le 15; $i++) {
    try {
        $response = Invoke-WebRequest -Uri $url -Method Get -ErrorAction Stop
        $remaining = $response.Headers["X-RateLimit-Remaining"]
        Write-Host "[Req $i] IP: 127.0.0.1 | Status: $($response.StatusCode) | Remaining: $remaining" -ForegroundColor Green
    }
    catch {
        $status = $_.Exception.Response.StatusCode.value__
        $retryAfter = $_.Exception.Response.Headers["Retry-After"]
        Write-Host "[Req $i] IP: 127.0.0.1 | Status: $status | LIMIT HIT! Retry-After: $($retryAfter)s" -ForegroundColor Red
    }
}
