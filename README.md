# orb

A packaging transpiler for Kubernetes package formats.

orb converts between Kubernetes packaging formats, including:

- **OLM registry+v1 bundles**
- **Helm charts**
- **Plain manifests**

## Overview

orb reads a source package in one format and produces an equivalent package in another format, enabling interoperability across the Kubernetes packaging ecosystem.

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
| `certProvider` | Certificate provider for webhooks: `""`, `"cert-manager"`, or `"service-ca"`. Only present if the bundle has webhooks. | `""` |
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
# Render with default values (AllNamespaces mode, no cert provider)
helm template my-release /tmp/chart --namespace operators

# Watch a single namespace
helm template my-release /tmp/chart --namespace operators --set watchNamespace=monitoring

# Enable cert-manager for webhook certificates
helm template my-release /tmp/chart --namespace operators --set certProvider=cert-manager

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
