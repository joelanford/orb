# orb

orb puts OLM's building blocks in your hands — resolve, render, and inspect operator bundles and catalogs, all client-side.

## Usage

### Render to plain manifests

```sh
orb render plain docker://quay.io/my/bundle:v1 -n operators
orb render plain docker://quay.io/my/bundle:v1 dir:/tmp/output -n operators
orb render plain docker://quay.io/my/bundle:v1 -n operators --cert-provider cert-manager
```

### Render to Helm chart

```sh
orb render helm docker://quay.io/my/bundle:v1 dir:/tmp/chart
```

The generated Helm chart supports install-time configuration via values:

| Value | Description | Default |
|-------|-------------|---------|
| `watchNamespace` | Namespace to watch. `""` = AllNamespaces; set to a namespace for OwnNamespace/SingleNamespace. | `""` |
| `certProvider` | Certificate provider for webhooks: `"cert-manager"` or `"service-ca"`. Required when the bundle has webhooks. | `"cert-manager"` |
| `deploymentConfig` | Deployment overrides following OLM `subscription.spec.config` semantics. | `{}` |

#### deploymentConfig fields

| Field | Merge behavior |
|-------|---------------|
| `env` | Override by name, append new |
| `envFrom` | Append |
| `volumes` | Append |
| `volumeMounts` | Append to all containers |
| `tolerations` | Append |
| `resources` | Replace |
| `nodeSelector` | Replace |
| `affinity` | Override non-nil sub-fields (nodeAffinity, podAffinity, podAntiAffinity) |
| `annotations` | Existing annotations take precedence |

#### Examples

```sh
# Render with default values (AllNamespaces mode)
helm template my-release /tmp/chart --namespace operators

# Watch a single namespace
helm template my-release /tmp/chart --namespace operators --set watchNamespace=monitoring

# Use service-ca instead of cert-manager for webhook certificates
helm template my-release /tmp/chart --namespace operators --set certProvider=service-ca

# Override deployment resources
helm template my-release /tmp/chart --namespace operators \
  --set deploymentConfig.resources.limits.cpu=2 \
  --set deploymentConfig.resources.limits.memory=8Gi
```

### Supported transports

Sources and destinations use skopeo-style transport prefixes:

| Transport | Description |
|-----------|-------------|
| `docker://` | Container registry |
| `oci:` | OCI image layout directory |
| `oci-archive:` | OCI image layout archive |
| `dir:` | Local directory |
| `tar:` | Tar archive (source only) |
| `stdout` | Standard output (plain format only) |

## License

[Apache License 2.0](LICENSE)
