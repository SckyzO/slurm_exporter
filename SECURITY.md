# Security Policy

## Reporting a vulnerability

Please report security issues privately through GitHub's
[private vulnerability reporting](https://github.com/SckyzO/slurm_exporter/security/advisories/new)
(repository **Security** tab → **Report a vulnerability**). This keeps the
report confidential until a fix is available.

Do not open a public issue or pull request for a suspected vulnerability.

Include, where possible: the affected version, the command(s) the exporter was
running, and the steps to reproduce. Reports are acknowledged and triaged on a
best-effort basis.

## Supported versions

| Version | Supported |
|---|---|
| Latest release | ✅ |
| Older releases | ❌ |

Only the latest release receives security fixes. This project is not actively
maintained for Slurm 25.11+, which natively supports OpenMetrics for Prometheus
— see [sckyzo/slurm_prometheus_exporter](https://github.com/sckyzo/slurm_prometheus_exporter/)
for new deployments.

## Verifying published artifacts

Released binaries and container images carry signed provenance (cosign keyless),
signed checksums, and CycloneDX SBOMs. Verification recipes are in the
[Security & supply chain](README.md#-security--supply-chain) section of the
README and in [`docker/README.md`](docker/README.md#supply-chain).
