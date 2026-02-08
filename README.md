# provider-orgmapper

A [Crossplane](https://crossplane.io/) Provider for managing multi-tenant observability infrastructure. This provider enables Kubernetes-native management of LGTM stack (Logs, Traces, Metrics, Profiles) tenants with automatic Grafana SSO integration.

## Overview

`provider-orgmapper` allows you to:

- Define tenants as Kubernetes Custom Resources
- Automatically sync tenant configurations to Grafana SSO org_mapping
- Manage viewer and editor group access per tenant
- Configure data retention policies for logs, metrics, traces, and profiles
- Track tenant state with drift detection

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Kubernetes Cluster                             │
│                                                                             │
│  ┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐        │
│  │  Tenant CR      │     │  Tenant CR      │     │  Tenant CR      │        │
│  │  (acme-corp)    │     │  (beta-inc)     │     │  (gamma-co)     │        │
│  └────────┬────────┘     └────────┬────────┘     └────────┬────────┘        │
│           │                       │                       │                 │
│           └───────────────────────┼───────────────────────┘                 │
│                                   │                                         │
│                                   ▼                                         │
│                    ┌──────────────────────────────┐                         │
│                    │     provider-orgmapper       │                         │
│                    │                              │                         │
│                    │  ┌────────────────────────┐  │                         │
│                    │  │   Tenant Controller    │  │                         │
│                    │  │   - Watch Tenant CRs   │  │                         │
│                    │  │   - Reconcile state    │  │                         │
│                    │  │   - Sync to Grafana    │  │                         │
│                    │  └───────────┬────────────┘  │                         │
│                    │              │               │                         │
│                    │  ┌───────────▼────────────┐  │                         │
│                    │  │   ProviderConfig       │  │                         │
│                    │  │   - Grafana URL        │  │                         │
│                    │  │   - SA Token (Secret)  │  │                         │
│                    │  └───────────┬────────────┘  │                         │
│                    └──────────────┼───────────────┘                         │
│                                   │                                         │
└───────────────────────────────────┼─────────────────────────────────────────┘
                                    │
                                    ▼
                    ┌───────────────────────────────┐
                    │         Grafana               │
                    │                               │
                    │  SSO Settings                 │
                    │  ┌─────────────────────────┐  │
                    │  │ org_mapping:            │  │
                    │  │  - acme-viewers:1       │  │
                    │  │  - acme-editors:1       │  │
                    │  │  - beta-viewers:2       │  │
                    │  │  - gamma-editors:3      │  │
                    │  └─────────────────────────┘  │
                    └───────────────────────────────┘
```

### Component Flow

```
┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐
│  Create  │    │  Watch   │    │ Observe  │    │   Sync   │
│ Tenant   │───▶│  Event   │───▶│ Grafana  │───▶│ org_map  │
│   CR     │    │          │    │  State   │    │          │
└──────────┘    └──────────┘    └──────────┘    └──────────┘
                                      │               │
                                      ▼               ▼
                               ┌──────────┐    ┌──────────┐
                               │  Drift   │    │  Update  │
                               │ Detected │───▶│  Status  │
                               └──────────┘    └──────────┘
```

## Installation

### Prerequisites

- Kubernetes cluster (v1.25+)
- Crossplane installed (v1.14+)
- Grafana instance with SSO configured
- Grafana Service Account token with admin permissions

### Install the Provider

```yaml
apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: provider-orgmapper
spec:
  package: xpkg.upbound.io/crossplane-contrib/provider-orgmapper:v0.1.0
```

## Usage

### 1. Create Grafana Credentials Secret

Store your Grafana service account token in a Kubernetes Secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: grafana-credentials
  namespace: crossplane-system
type: Opaque
stringData:
  token: "glsa_xxxxxxxxxxxxxxxxxxxxxxxxxxxx"
```

### Using Basic Authentication

If you prefer to use a username and password (Basic Auth), create a Secret with a JSON string containing "username" and "password" fields:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: grafana-basic-auth
  namespace: crossplane-system
type: Opaque
stringData:
  credentials: |
    {
      "username": "admin",
      "password": "feature-rich-password"
    }
```

Then update your ProviderConfig to reference this secret key:

```yaml
apiVersion: orgmapper.crossplane.io/v1alpha1
kind: ProviderConfig
metadata:
  name: default
  namespace: crossplane-system
spec:
  grafanaUrl: "https://grafana.example.com"
  credentials:
    source: Secret
    secretRef:
      namespace: crossplane-system
      name: grafana-basic-auth
      key: credentials
```

### 2. Configure the Provider

Create a `ProviderConfig` to connect to your Grafana instance:

```yaml
apiVersion: orgmapper.crossplane.io/v1alpha1
kind: ProviderConfig
metadata:
  name: default
  namespace: crossplane-system
spec:
  grafanaUrl: "https://grafana.example.com"
  credentials:
    source: Secret
    secretRef:
      namespace: crossplane-system
      name: grafana-credentials
      key: token
```

For cluster-wide configuration, use `ClusterProviderConfig`:

```yaml
apiVersion: orgmapper.crossplane.io/v1alpha1
kind: ClusterProviderConfig
metadata:
  name: default
spec:
  grafanaUrl: "https://grafana.example.com"
  credentials:
    source: Secret
    secretRef:
      namespace: crossplane-system
      name: grafana-credentials
      key: token
```

### 3. Create Tenants

Define tenants as Kubernetes resources:

```yaml
apiVersion: tenant.orgmapper.crossplane.io/v1alpha1
kind: Tenant
metadata:
  name: acme-corp
  namespace: default
spec:
  forProvider:
    # Unique tenant identifier
    tenantId: acme-corp

    # Grafana organization ID to map this tenant to
    orgId: "1"

    # Tenant administrators (e.g., GitHub usernames)
    admins:
      - alice
      - bob

    # Groups that get Viewer role in Grafana
    viewerGroups:
      - acme-developers
      - acme-oncall

    # Groups that get Editor role in Grafana
    editorGroups:
      - acme-sre
      - acme-platform

    # Data retention configuration
    retention:
      logs: "30d"
      metrics: "90d"
      traces: "14d"
      profiles: "7d"

  providerConfigRef:
    name: default
```

### 4. Verify Tenant Status

Check that tenants are synced:

```bash
kubectl get tenants -A
```

Output:
```
NAMESPACE   NAME        READY   SYNCED   EXTERNAL-NAME   TENANT-ID    ORG-ID   AGE
default     acme-corp   True    True     acme-corp       acme-corp    1        5m
default     beta-inc    True    True     beta-inc        beta-inc     2        3m
```

Get detailed status:

```bash
kubectl describe tenant acme-corp
```

## API Reference

### Tenant

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `spec.forProvider.tenantId` | string | Yes | Unique identifier for the tenant |
| `spec.forProvider.orgId` | string | Yes | Grafana organization ID |
| `spec.forProvider.admins` | []string | No | List of tenant administrators |
| `spec.forProvider.viewerGroups` | []string | No | Groups with Viewer role |
| `spec.forProvider.editorGroups` | []string | No | Groups with Editor role |
| `spec.forProvider.retention.logs` | string | No | Log retention (e.g., "30d") |
| `spec.forProvider.retention.metrics` | string | No | Metrics retention |
| `spec.forProvider.retention.traces` | string | No | Traces retention |
| `spec.forProvider.retention.profiles` | string | No | Profiles retention |

### ProviderConfig

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `spec.grafanaUrl` | string | Yes | Grafana instance URL |
| `spec.credentials.source` | string | Yes | Credential source ("Secret") |
| `spec.credentials.secretRef` | object | Yes | Reference to Secret with token |

### Retention Duration Format

Retention values support the following suffixes:
- `h` - hours (e.g., "24h")
- `d` - days (e.g., "30d")
- `w` - weeks (e.g., "4w")
- `m` - months (e.g., "3m")
- `y` - years (e.g., "1y")

## Examples

### Multi-Environment Setup

```yaml
# Production tenant with long retention
apiVersion: tenant.orgmapper.crossplane.io/v1alpha1
kind: Tenant
metadata:
  name: myapp-prod
spec:
  forProvider:
    tenantId: myapp-prod
    orgId: "10"
    viewerGroups:
      - myapp-developers
    editorGroups:
      - myapp-sre
    retention:
      logs: "90d"
      metrics: "1y"
      traces: "30d"
      profiles: "14d"
  providerConfigRef:
    name: default
---
# Staging tenant with shorter retention
apiVersion: tenant.orgmapper.crossplane.io/v1alpha1
kind: Tenant
metadata:
  name: myapp-staging
spec:
  forProvider:
    tenantId: myapp-staging
    orgId: "11"
    viewerGroups:
      - myapp-developers
    editorGroups:
      - myapp-developers
    retention:
      logs: "7d"
      metrics: "30d"
      traces: "7d"
      profiles: "3d"
  providerConfigRef:
    name: default
```

### Team-Based Access Control

```yaml
apiVersion: tenant.orgmapper.crossplane.io/v1alpha1
kind: Tenant
metadata:
  name: platform-team
spec:
  forProvider:
    tenantId: platform
    orgId: "5"
    admins:
      - platform-lead
    viewerGroups:
      - engineering-all
      - support-tier2
    editorGroups:
      - platform-engineers
      - sre-team
    retention:
      logs: "60d"
      metrics: "180d"
      traces: "30d"
      profiles: "14d"
  providerConfigRef:
    name: default
```

## Developing

### Prerequisites

- Go 1.24+
- Docker
- kubectl
- KinD (for local development)

### Quick Start

```bash
# Initialize build submodule
make submodules

# Run linters and tests
make reviewable

# Build the provider
make build

# Run locally with a KinD cluster
make dev
```

### Running Tests

```bash
make test
```

### Code Generation

After modifying API types, regenerate code:

```bash
make generate
```

### Local Development Cluster

Create a local cluster with the provider installed:

```bash
make dev
```

This creates a KinD cluster, installs Crossplane, and deploys the provider.

### Adding New Resource Types

```bash
export provider_name=OrgMapper
export group=mygroup
export type=MyResource
make provider.addtype provider=${provider_name} group=${group} kind=${type}
```

## Troubleshooting

### Tenant stuck in "Syncing" state

1. Check provider logs:
   ```bash
   kubectl logs -n crossplane-system -l pkg.crossplane.io/provider=provider-orgmapper
   ```

2. Verify ProviderConfig credentials:
   ```bash
   kubectl get providerconfig default -o yaml
   ```

3. Test Grafana connectivity:
   ```bash
   curl -H "Authorization: Bearer $TOKEN" https://grafana.example.com/api/org
   ```

### Drift detected but not correcting

The provider polls Grafana every minute. Check that:
- The service account has admin permissions
- SSO settings in Grafana are not locked by another process

### Permission denied errors

Ensure the Grafana service account token has:
- `Admin` role in the target organizations
- Permission to modify SSO settings

## Contributing

Refer to Crossplane's [CONTRIBUTING.md](https://github.com/crossplane/crossplane/blob/master/CONTRIBUTING.md) for contribution guidelines. The [Provider Development Guide](https://github.com/crossplane/crossplane/blob/master/contributing/guide-provider-development.md) provides additional context.

## License

Apache 2.0 - See [LICENSE](LICENSE) for details.
