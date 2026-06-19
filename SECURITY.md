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

## Security practices

Several checks run automatically so issues are caught before they ship:

- `govulncheck` runs on every pull request and weekly, and reports only the
  vulnerabilities reachable from the code.
- CodeQL (extended query suite) and `gosec` check the Go code for vulnerable
  patterns.
- Container images are scanned for known CVEs with Trivy and run as a non-root
  user.
- Third-party GitHub Actions are pinned to commit SHAs, workflow tokens are kept
  to least privilege, and `master` merges only once the security checks pass.
- Dependabot proposes dependency updates weekly, with a short cooldown so a
  freshly published release is not pulled in the same day.

Released binaries and images are signed; see
[Verifying published artifacts](#verifying-published-artifacts) below.

## Verifying published artifacts

Released binaries and container images carry signed provenance (cosign keyless),
signed checksums, and CycloneDX SBOMs. Verification recipes are in the
[Security & supply chain](README.md#-security--supply-chain) section of the
README and in [`docker/README.md`](docker/README.md#supply-chain).
