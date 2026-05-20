# sunny — Helm chart

Self-hosted, open-source observability platform on its way to becoming a
self-hosted Databricks. This chart installs the **v0.1 → v1.0 single-binary
build** plus optional integrations: Prometheus ServiceMonitor, NetworkPolicy,
PodDisruptionBudget, and SSO via OIDC.

## Quick start

```sh
helm install sunny ./charts/sunny
```

By default Sunny boots with no auth and a 10 GiB PVC for DuckDB at
`/data`. Visit `http://<service>:3000`.

## Hardened install

```sh
helm install sunny ./charts/sunny \
  --set auth.passwordHash="$(docker run --rm ghcr.io/sunny/sunny:latest sunny-cli hash-password 'mypw')" \
  --set auth.sessionKey="$(openssl rand -hex 32)" \
  --set ingress.enabled=true \
  --set ingress.hosts[0].host=sunny.example.com \
  --set ingress.hosts[0].paths[0].path=/ \
  --set ingress.hosts[0].paths[0].pathType=Prefix \
  --set persistence.size=50Gi \
  --set resources.limits.cpu=2 \
  --set resources.limits.memory=4Gi
```

## SSO (OIDC, Phase 0.6)

```sh
helm install sunny ./charts/sunny \
  --set oidc.issuer=https://acme.okta.com \
  --set oidc.clientId=0oa... \
  --set oidc.clientSecret=$OKTA_SECRET \
  --set oidc.redirectUrl=https://sunny.acme.com/api/auth/oidc/callback
```

## Prometheus integration

Two options:

1. **Operator-managed (recommended):** install kube-prometheus-stack, then:
   ```sh
   helm upgrade sunny ./charts/sunny --set serviceMonitor.enabled=true
   ```
2. **Direct scrape:** add a `prometheus_io_scrape: "true"` annotation in
   `service.annotations` and configure your scrape job to find it.

## Alerts (Phase 0.5)

```sh
helm upgrade sunny ./charts/sunny \
  --set alerts.slackUrl=https://hooks.slack.com/services/T0/.../... \
  --set alerts.webhookUrl=https://ops.example.com/sunny-webhook
```

Triggered alerts that exhaust retry attempts are written to
`/data/alerts-dlq.jsonl` and exposed at `/api/v1/alerts/deadletters`. Use
`sunny-cli alerts deadletters` to inspect them.

## Air-gapped install

1. Pull and push the image to your internal registry:
   ```sh
   docker pull ghcr.io/sunny/sunny:latest
   docker tag ghcr.io/sunny/sunny:latest registry.acme.com/sunny:1.0.0
   docker push registry.acme.com/sunny:1.0.0
   ```
2. Pin to a sha256 digest for supply-chain attestation:
   ```sh
   helm install sunny ./charts/sunny \
     --set image.repository=registry.acme.com/sunny \
     --set image.digest=sha256:<full-digest> \
     --set image.pullSecrets[0].name=acme-pull \
     --set image.pullPolicy=IfNotPresent
   ```
3. (Optional) Vendor the chart itself by `helm package` + serving from
   an internal chart museum / OCI registry.

## Security defaults

The chart ships with hardened defaults:

- `runAsNonRoot: true`, `runAsUser: 65532` (distroless `nonroot`).
- `readOnlyRootFilesystem: true` with `/tmp` mounted as an in-memory tmpfs.
- All Linux capabilities dropped.
- `seccompProfile: RuntimeDefault`.
- `allowPrivilegeEscalation: false`.

For multi-tenant clusters, also enable `networkPolicy.enabled` and supply
ingress rules limiting which namespaces can reach Sunny.

## Probes

Liveness and readiness use `/healthz` and `/readyz` — Kubernetes-native
endpoints outside `/api` so they're never gated by auth, CORS, or rate
limits. Override paths in `values.yaml` if you front Sunny behind a path
prefix.

## Upgrade

```sh
helm upgrade sunny ./charts/sunny --reuse-values
```

The `checksum/secret` annotation on the Deployment forces a pod restart
whenever any secret value changes, so password/OIDC rotations propagate
without manual intervention.

## Limitations (v0.1 / v1.0)

- **Single replica only.** DuckDB is single-writer; horizontal scaling
  requires the Iceberg backend (Phase 1+) or ClickHouse mode (Phase 2.5+).
  When that ships, `replicaCount` becomes meaningful and PDB defaults flip.
- **No built-in TLS termination.** Front Sunny with an ingress controller
  or a reverse proxy; `ingress.tls` configures it for the standard cases.
- **No backup automation in-cluster.** Run `sunny-cli backup` from a
  CronJob (template TBD in Phase 0.10).

## Values reference

See [`values.yaml`](./values.yaml) for the full annotated list.
