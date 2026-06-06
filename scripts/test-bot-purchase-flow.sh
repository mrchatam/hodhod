#!/usr/bin/env bash
# Manual smoke script for bot purchase flows (wallet + card receipt).
set -euo pipefail
BASE="${PUBLIC_BASE_URL:-http://127.0.0.1:8080}"
echo "Hodhod bot purchase flow smoke test"
echo "Base URL: $BASE"
echo "1. Ensure a bot exists with active plans and panel assignment"
echo "2. Wallet flow: /start -> plans -> pay wallet -> receive sub link"
echo "3. Card flow: /start -> plans -> pay card -> upload receipt -> approve in panel or TG"
echo "4. Verify /payments/pending shows order type and end-user context"
echo "5. Mini App: POST $BASE/api/miniapp/{publicID}/orders/card with plan_id + receipt_ref"
curl -sf "$BASE/healthz" >/dev/null && echo "healthz: ok" || echo "healthz: fail (is server running?)"
