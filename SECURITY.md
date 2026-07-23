# Security

Report vulnerabilities privately to the owning organization. Do not open a
public issue containing exploit details or credentials.

## Baseline rules

- Keep Go and dependencies supported and patched.
- Use parameterized SQL and strict JSON decoding.
- Terminate TLS at a trusted ingress or load balancer.
- Configure trusted proxy headers at deployment time; do not trust arbitrary
  forwarded IP headers in the application by default.
- Keep secrets in the deployment platform's secret manager.
- Apply least-privilege database credentials and network policies.
- Avoid logging authentication data, personal data, and payment data.

Authentication and authorization are application-specific and intentionally not
implemented in this template.

## Service extraction

Moving a module behind a network interface creates a new trust boundary. Before
cutover, define workload identity, per-operation authorization, encrypted
transport, network policy, least-privilege service and database credentials,
rotation ownership, and incident response. Do not assume that an internal
network is trusted. See `docs/MICROSERVICES.md` for the full extraction gate.
