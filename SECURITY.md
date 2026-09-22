# Security policy

## Reporting a vulnerability

Please do not open a public issue for a suspected vulnerability. Use GitHub's private vulnerability reporting for this repository and include affected versions, impact, reproduction steps, and any suggested mitigation. Maintainers will acknowledge a complete report as soon as practical and coordinate disclosure after a fix is available.

## Deployment guidance

KEDA Dashboard is an operational interface and does not include user authentication. Expose it only on trusted networks or place it behind an authenticated reverse proxy or ingress. Use TLS for any traffic outside the cluster.

The supplied RBAC intentionally:

- permits only `get`, `list`, and `watch`;
- grants no access to Secrets;
- grants no wildcard resources or verbs;
- limits reads to KEDA ScaledObjects/ScaledJobs, related HPAs/workloads, and Kubernetes events.

Do not broaden these permissions without reviewing the data exposure. Kubernetes events, labels, annotations, and KEDA trigger metadata can contain operationally sensitive values. The YAML view recursively redacts common credential-shaped keys, but operators should still avoid placing credentials directly in custom resources and use Kubernetes Secrets with TriggerAuthentication instead.

Keep the image, Kubernetes, KEDA, and ingress/authentication components patched. The default container runs as non-root with a read-only filesystem, dropped capabilities, and RuntimeDefault seccomp.

## Supported versions

Until the first stable release, only the latest release receives security fixes.
