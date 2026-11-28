#!/usr/bin/env bash
# Deploy a pinned image SHA to prod. Usage: deploy/scripts/deploy.sh <git-sha>
set -euo pipefail

SHA="${1:?usage: deploy.sh <git-sha>}"
OVERLAY="deploy/k8s/overlays/prod"
DEPLOYMENTS=(otp-api auth-svc otp-dispatcher link-svc link-dispatcher)

cd "$(git rev-parse --show-toplevel)"

for svc in "${DEPLOYMENTS[@]}" seed; do
  ( cd "$OVERLAY" && kustomize edit set image "$svc=ghcr.io/diikhanh/worklane-$svc:$SHA" )
done

echo ">> Applying prod overlay pinned to $SHA"
kubectl apply -k "$OVERLAY"
for deployment in "${DEPLOYMENTS[@]}"; do
  kubectl -n worklane rollout status "deploy/$deployment" --timeout=120s
done

echo ">> Deployed $SHA. Commit the overlay change so git records what is running:"
echo "   git add $OVERLAY/kustomization.yaml && git commit -m 'deploy: prod -> $SHA'"
