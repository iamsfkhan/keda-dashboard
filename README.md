# KEDA Dashboard

A read-only dashboard for observing [KEDA](https://keda.sh/) resources across a Kubernetes cluster. A Go API and embedded React SPA ship as one small container.

## Features

- Cluster overview, status-filtered resource lists, and detail pages for ScaledObjects and ScaledJobs
- Conditions, replica state, target workload, authentication references, related HPA, events, and redacted YAML
- Live Server-Sent Events with polling fallback
- Optional one-hour Prometheus metric history
- In-cluster authentication with local kubeconfig fallback for development
- A deterministic demo mode requiring no Kubernetes configuration, credentials, or network access
- Read-only RBAC, no mutation endpoints, and no Kubernetes Secret access

## Run the demo

Demo mode uses built-in synthetic data and never creates a Kubernetes client:

```bash
docker run --rm -p 8080:8080 \
  -e DEMO_MODE=true \
  iamsfkhan/keda-dashboard:0.2.0
```

To build from source instead, run `docker build -t keda-dashboard:local .` and replace the image name above with `keda-dashboard:local`.

Open <http://localhost:8080>. The demo includes healthy, active, paused, and unhealthy resources across multiple namespaces, along with conditions, events, relationships, triggers, and safe YAML.

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `DEMO_MODE` | `false` | Set exactly to `true` to use deterministic synthetic data and disable all Kubernetes access |
| `LISTEN_ADDRESS` | `:8080` | HTTP bind address |
| `LOG_LEVEL` | `info` | Set to `debug` for verbose structured logs |
| `PROMETHEUS_URL` | empty | Prometheus base URL, for example `http://prometheus.monitoring.svc:9090` |
| `KUBECONFIG` | `~/.kube/config` | Local Kubernetes config path outside the cluster |

In normal mode the backend first attempts in-cluster service-account authentication, then `$KUBECONFIG`, then `~/.kube/config`. Helm deployments use normal mode.

## Deploy to Amazon EKS with Helm

Published artifacts:

- Container image: `docker.io/iamsfkhan/keda-dashboard:0.2.0`
- Helm chart: `oci://registry-1.docker.io/iamsfkhan/keda-dashboard-chart` version `0.2.0`

### Prerequisites

- An EKS cluster with [KEDA installed](https://keda.sh/docs/latest/deploy/)
- `aws`, `kubectl`, and Helm 3 configured for the target cluster
- EKS nodes able to pull the public `iamsfkhan/keda-dashboard:0.2.0` image, or your own registry mirror
- For ingress: [AWS Load Balancer Controller](https://kubernetes-sigs.github.io/aws-load-balancer-controller/latest/deploy/installation/) installed with its required IAM permissions
- For TLS: an ACM certificate in the same AWS region as the ALB

Confirm the target identity and KEDA API before installing:

```bash
aws eks update-kubeconfig --name my-eks-cluster --region us-east-1
kubectl config current-context
kubectl get crd scaledobjects.keda.sh scaledjobs.keda.sh
```

### Install the published chart

Install the public OCI chart and container image without cloning this repository:

```bash
helm upgrade --install keda-dashboard \
  oci://registry-1.docker.io/iamsfkhan/keda-dashboard-chart \
  --version 0.2.0 \
  --namespace keda-dashboard \
  --create-namespace \
  --set image.repository=iamsfkhan/keda-dashboard \
  --set image.tag=0.2.0
```

The chart creates a dedicated ServiceAccount by default and binds it to a cluster-wide, read-only ClusterRole so resources in all namespaces are visible. It grants only `get`, `list`, and `watch` for ScaledObjects, ScaledJobs, HPAs, selected workload metadata, and Events. It does **not** grant access to Secrets.

For production environments that require ECR, mirror the same tag into ECR and override `image.repository`. For private registries, configure `imagePullSecrets`.

If service-account management is handled elsewhere:

```bash
helm upgrade --install keda-dashboard \
  oci://registry-1.docker.io/iamsfkhan/keda-dashboard-chart \
  --version 0.2.0 \
  --namespace keda-dashboard \
  --create-namespace \
  --set serviceAccount.create=false \
  --set serviceAccount.name=keda-dashboard \
  --set image.repository=iamsfkhan/keda-dashboard \
  --set image.tag=0.2.0
```

The named ServiceAccount must already exist. The chart still creates its read-only ClusterRole and binding.

### Private access with ClusterIP and port-forward

The Service defaults to `ClusterIP`, which does not expose the dashboard outside the cluster:

```bash
kubectl -n keda-dashboard rollout status deployment/keda-dashboard-keda-dashboard
kubectl -n keda-dashboard port-forward service/keda-dashboard-keda-dashboard 8080:80
```

Open <http://localhost:8080>. These names use release `keda-dashboard`; find overridden names with `kubectl -n keda-dashboard get deployment,service`.

### Internal ALB ingress

Create `values-eks.yaml`:

```yaml
image:
  repository: iamsfkhan/keda-dashboard
  tag: 0.2.0

ingress:
  enabled: true
  className: alb
  annotations:
    alb.ingress.kubernetes.io/scheme: internal
    alb.ingress.kubernetes.io/target-type: ip
    alb.ingress.kubernetes.io/listen-ports: '[{"HTTP":80}]'
    alb.ingress.kubernetes.io/healthcheck-path: /healthz
    alb.ingress.kubernetes.io/success-codes: "200"
  hosts:
    - host: keda-dashboard.internal.example.com
      paths:
        - path: /
          pathType: Prefix
```

Install or apply the update:

```bash
helm upgrade --install keda-dashboard \
  oci://registry-1.docker.io/iamsfkhan/keda-dashboard-chart \
  --version 0.2.0 \
  --namespace keda-dashboard \
  --create-namespace \
  -f values-eks.yaml
kubectl -n keda-dashboard get ingress keda-dashboard-keda-dashboard
```

Keep the ALB internal. The dashboard does not implement user authentication or authorization. **Do not expose it publicly without an authentication layer, restrictive network controls, and TLS.** Consider an OIDC-capable proxy, ALB authentication, security-group restrictions, and private DNS.

### TLS with ACM and DNS

Add the certificate ARN and HTTPS listener to the ingress annotations:

```yaml
ingress:
  annotations:
    alb.ingress.kubernetes.io/scheme: internal
    alb.ingress.kubernetes.io/target-type: ip
    alb.ingress.kubernetes.io/listen-ports: '[{"HTTPS":443}]'
    alb.ingress.kubernetes.io/certificate-arn: arn:aws:acm:us-east-1:123456789012:certificate/00000000-0000-0000-0000-000000000000
    alb.ingress.kubernetes.io/ssl-redirect: "443"
    alb.ingress.kubernetes.io/healthcheck-path: /healthz
```

After the ingress reports an ALB hostname, create a private Route 53 alias record for `keda-dashboard.internal.example.com`. With ExternalDNS, add its hostname annotation according to your ExternalDNS policy. ACM validation and the Route 53 hosted zone must cover the selected hostname.

### Prometheus

Set a cluster-reachable Prometheus URL:

```yaml
prometheus:
  url: http://prometheus-server.monitoring.svc.cluster.local:80
```

Prometheus is optional. Query failures degrade to an empty metric history and do not block resource pages or readiness.

### Verify

```bash
kubectl -n keda-dashboard get deployment,pod,service,ingress
kubectl -n keda-dashboard logs deployment/keda-dashboard-keda-dashboard
kubectl -n keda-dashboard auth can-i list scaledobjects.keda.sh \
  --as=system:serviceaccount:keda-dashboard:keda-dashboard-keda-dashboard
kubectl -n keda-dashboard auth can-i get secrets \
  --as=system:serviceaccount:keda-dashboard:keda-dashboard-keda-dashboard
```

The first authorization check should return `yes`; the Secret check must return `no`. Probe `/healthz` for process health and `/readyz` for Kubernetes API connectivity.

### Upgrade and uninstall

```bash
helm upgrade keda-dashboard \
  oci://registry-1.docker.io/iamsfkhan/keda-dashboard-chart \
  --version 0.2.0 \
  --namespace keda-dashboard \
  --reuse-values \
  --set image.tag=0.2.0

helm uninstall keda-dashboard --namespace keda-dashboard
kubectl delete namespace keda-dashboard
```

Review `helm diff` or `helm template` output before production upgrades if those tools are part of your release process.

## Security model

The dashboard is read-only by design. Its chart has no write verbs and no Secret permissions. Safe YAML recursively redacts keys that look sensitive because KEDA trigger metadata can contain inline credentials even when Secret reads are forbidden. See [SECURITY.md](SECURITY.md).

Ingress is a separate security boundary: the application has no built-in login. Restrict it to trusted networks and add authentication before allowing user access.

## Local development

Prerequisites: Go 1.23+, Node.js 22+, and npm.

```bash
npm ci --prefix frontend
make frontend-build
DEMO_MODE=true go run ./cmd/dashboard
```

For real local-cluster behavior, omit `DEMO_MODE`; the backend will use the configured kubeconfig.

```bash
make test
make lint
make helm-lint
make validate-rbac
make build
make docker
```

To package and publish a release, keep the image and chart in separate OCI repositories:

```bash
docker buildx build --platform linux/amd64,linux/arm64 \
  --tag iamsfkhan/keda-dashboard:0.2.0 \
  --push .
helm package charts/keda-dashboard
helm push keda-dashboard-chart-0.2.0.tgz \
  oci://registry-1.docker.io/iamsfkhan
```

The OpenAPI specification is in [`api/openapi.yaml`](api/openapi.yaml). Main packages:

- `cmd/dashboard`: runtime selection, process setup, and graceful shutdown
- `internal/demo`: deterministic cluster-free demo provider
- `internal/kube`: Kubernetes discovery, querying, enrichment, and YAML sanitization
- `internal/server`: HTTP API, SSE, and SPA fallback
- `internal/prometheus`: optional bounded Prometheus client
- `frontend`: React/Vite source embedded into the binary
- `charts/keda-dashboard`: least-privilege Helm deployment

## License

Apache License 2.0. See [LICENSE](LICENSE).
