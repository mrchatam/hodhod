#!/usr/bin/env bash
set -euo pipefail

DOMAIN="${1:-}"
HTTP_PORT="${2:-8080}"

if [[ -z "$DOMAIN" ]]; then
  echo "Usage: $0 SELLER_DOMAIN [HTTP_PORT]"
  exit 1
fi

TEMPLATE="$(dirname "$0")/../deploy/nginx/hodhod-seller-domain.conf.example"
OUT="/etc/nginx/sites-available/hodhod-seller-${DOMAIN}.conf"

sudo sed "s/__SELLER_DOMAIN__/${DOMAIN}/g; s/__HTTP_PORT__/${HTTP_PORT}/g" "$TEMPLATE" | sudo tee "$OUT" >/dev/null
sudo ln -sf "$OUT" "/etc/nginx/sites-enabled/hodhod-seller-${DOMAIN}.conf"
sudo nginx -t
sudo certbot --nginx -d "$DOMAIN" --redirect --non-interactive --agree-tos -m "${CERTBOT_EMAIL:-admin@${DOMAIN}}" || true
sudo systemctl reload nginx
echo "Seller domain $DOMAIN configured. Verify DNS in master panel, then enable domain."
