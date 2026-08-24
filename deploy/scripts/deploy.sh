#!/usr/bin/env bash
# Deploy a pinned image SHA to prod. Usage: deploy/scripts/deploy.sh <git-sha>
set -euo pipefail

SHA="${1:?usage: deploy.sh <git-sha>}"
OVERLAY="deploy/k8s/overlays/prod"
SVCS=(otp-api auth-svc otp-dispatcher seed)

cd "$(git rev-parse --show-toplevel)"

for svc in "${SVCS[@]}"; do
  ( cd "$OVERLAY" && kustomize edit set image "$svc=ghcr.io/duykhanh/worklane-$svc:$SHA" )
done

echo ">> Applying prod overlay pinned to $SHA"
kubectl apply -k "$OVERLAY"
kubectl -n worklane rollout status deploy/otp-api --timeout=120s
kubectl -n worklane rollout status deploy/auth-svc --timeout=120s
kubectl -n worklane rollout status deploy/otp-dispatcher --timeout=120s

echo ">> Deployed $SHA. Commit the overlay change so git records what is running:"
echo "   git add $OVERLAY/kustomization.yaml && git commit -m 'deploy: prod -> $SHA'"
