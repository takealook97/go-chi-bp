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

