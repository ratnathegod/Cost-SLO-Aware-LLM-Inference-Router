#!/bin/bash

# Demo script for llm-router admin API
# Run this after starting the server with ADMIN_TOKEN=demo-token ENABLE_MOCK_PROVIDER=1

set -e

BASE_URL="http://localhost:8080"
ADMIN_TOKEN="demo-token"

echo "=== Admin API Demo ==="

echo -e "\n1. Testing admin status endpoint..."
curl -s -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$BASE_URL/v1/admin/status" | jq .

echo -e "\n2. Testing canary status..."
curl -s -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$BASE_URL/v1/admin/canary/status" | jq .

echo -e "\n3. Advancing canary stage (forced)..."
curl -s -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"force": true}' \
  "$BASE_URL/v1/admin/canary/advance"

echo -e "\n4. Checking canary status after advance..."
curl -s -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$BASE_URL/v1/admin/canary/status" | jq .

echo -e "\n5. Rolling back canary..."
curl -s -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$BASE_URL/v1/admin/canary/rollback"

echo -e "\n6. Checking canary status after rollback..."
curl -s -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$BASE_URL/v1/admin/canary/status" | jq .

echo -e "\n7. Updating default policy..."
curl -s -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"default_policy": "slo_burn_aware"}' \
  "$BASE_URL/v1/admin/policy"

echo -e "\n8. Checking updated status..."
curl -s -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$BASE_URL/v1/admin/status" | jq '.default_policy'

echo -e "\n9. Testing unauthorized access (should fail)..."
curl -s -w "\nHTTP Status: %{http_code}\n" \
  "$BASE_URL/v1/admin/status" | head -1

echo -e "\n10. Testing providers reload (not implemented)..."
curl -s -w "\nHTTP Status: %{http_code}\n" \
  -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$BASE_URL/v1/admin/providers/reload" | head -1

echo -e "\nAdmin API demo complete!"