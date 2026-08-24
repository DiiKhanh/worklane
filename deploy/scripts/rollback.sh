#!/usr/bin/env bash
# Roll prod back. Usage:
#   rollback.sh              -> kubectl rollout undo (last-known-good, fastest)
#   rollback.sh <git-sha>    -> re-deploy a specific previous SHA
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

if [[ $# -eq 0 ]]; then
  for d in otp-api auth-svc otp-dispatcher; do
    echo ">> rollout undo deploy/$d"
    kubectl -n worklane rollout undo "deploy/$d"
    kubectl -n worklane rollout status "deploy/$d" --timeout=120s
  done
  echo ">> Rolled back to previous ReplicaSet. Reconcile the overlay tag in git to match."
else
  exec deploy/scripts/deploy.sh "$1"
fi
