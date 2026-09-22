#!/usr/bin/env sh
set -eu

chart="${1:-charts/keda-dashboard}"
rendered="$(mktemp)"
trap 'rm -f "$rendered"' EXIT

helm template rbac-test "$chart" >"$rendered"

if grep -Eq 'resources:.*(secrets|\*)' "$rendered"; then
  echo "RBAC validation failed: Secret or wildcard resource access found" >&2
  exit 1
fi
if grep -Eq 'verbs:.*(create|update|patch|delete|deletecollection|\*)' "$rendered"; then
  echo "RBAC validation failed: mutating or wildcard verb found" >&2
  exit 1
fi
if ! grep -q 'resources: \["scaledobjects", "scaledjobs"\]' "$rendered"; then
  echo "RBAC validation failed: KEDA resources missing" >&2
  exit 1
fi

echo "RBAC is read-only and has no Secret access"
