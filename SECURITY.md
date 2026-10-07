# Security Policy

## Supported versions

Security fixes are provided for the latest release. Please update to the newest version
before reporting an issue.

## Reporting a vulnerability

Please **do not open a public issue** for security problems. Report them privately via
[GitHub Security Advisories](https://github.com/mrcdlm/labdeck/security/advisories/new)
("Report a vulnerability").

Include a description of the issue, the affected version and, if possible, steps to
reproduce. You will receive a response as soon as possible; fixes are published as a new
release together with an advisory.

## Deployment recommendations

- Do not expose the web interface directly to the internet; use a reverse proxy with TLS or
  a Cloudflare Tunnel, ideally with Cloudflare Access in front.
- Use a long, random `APP_PASSWORD` and keep `LABDECK_SECRET_KEY` secret – it decrypts all
  stored credentials.
- Give integrations read-only credentials where possible (Healthchecks.io read-only key,
  PBS token with DatastoreAudit only).
- Keep `.env` private (permissions `600`) and never commit it.
