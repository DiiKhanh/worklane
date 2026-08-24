#!/usr/bin/env bash
# Prove the latest R2 backup restores. Requires: aws cli + a scratch MySQL
# (docker) locally, and the r2-backup env vars exported.
set -euo pipefail

: "${R2_ENDPOINT:?}" "${R2_BUCKET:?}"
LATEST=$(aws s3 ls "s3://$R2_BUCKET/" --endpoint-url "$R2_ENDPOINT" \
  | awk '{print $4}' | sort | tail -1)
echo ">> latest backup: $LATEST"
aws s3 cp "s3://$R2_BUCKET/$LATEST" "/tmp/$LATEST" --endpoint-url "$R2_ENDPOINT"

docker run -d --rm --name restore-drill -e MYSQL_ROOT_PASSWORD=secret \
  -p 3307:3306 mysql:8.0
until docker exec restore-drill mysqladmin ping -psecret --silent 2>/dev/null; do sleep 2; done

gunzip -c "/tmp/$LATEST" | docker exec -i restore-drill mysql -uroot -psecret
echo ">> row counts after restore:"
docker exec restore-drill mysql -uroot -psecret -e \
  "SELECT 'tenants', COUNT(*) FROM identity.tenants
   UNION SELECT 'otp_requests', COUNT(*) FROM otp.otp_requests;"
docker stop restore-drill
echo ">> restore drill OK"
