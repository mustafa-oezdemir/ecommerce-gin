#!/bin/sh
set -eu

grafana_url="${GRAFANA_URL:-http://grafana:3000}"
auth="${GRAFANA_ADMIN_USER:?GRAFANA_ADMIN_USER is required}:${GRAFANA_ADMIN_PASSWORD:?GRAFANA_ADMIN_PASSWORD is required}"
payload='{"name":"PehliOne Operations","interval":"15s","items":[{"type":"dashboard_by_uid","value":"pehlione-overview","order":1,"title":"01 - PehliOne System Overview"},{"type":"dashboard_by_uid","value":"pehlione-ecommerce","order":2,"title":"02 - E-Commerce Operations"},{"type":"dashboard_by_uid","value":"pehlione-shipping","order":3,"title":"03 - Shipping Operations"},{"type":"dashboard_by_uid","value":"pehlione-integration","order":4,"title":"04 - E-Commerce ↔ Shipping Integration"},{"type":"dashboard_by_uid","value":"pehlione-infra","order":5,"title":"05 - Infrastructure & Reliability"},{"type":"dashboard_by_uid","value":"pehlione-security","order":6,"title":"06 - Security & Authentication"}]}'

response="$(curl --fail --silent --show-error --user "$auth" "$grafana_url/api/playlists?query=PehliOne%20Operations")"
uid="$(printf '%s' "$response" | sed -n 's/.*"uid":"\([^"]*\)".*/\1/p' | head -n 1)"

if [ -n "$uid" ]; then
  curl --fail --silent --show-error --user "$auth" -H 'Content-Type: application/json' -X PUT --data "$payload" "$grafana_url/api/playlists/$uid" >/dev/null
else
  curl --fail --silent --show-error --user "$auth" -H 'Content-Type: application/json' -X POST --data "$payload" "$grafana_url/api/playlists" >/dev/null
fi

printf '%s\n' 'PehliOne Operations playlist provisioned.'
