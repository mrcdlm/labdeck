# Changelog

All notable changes to this project are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.0] – 2026-10-07

### Added
- Integrations: data sources are added, tested, edited, paused and deleted in the web interface
  (Settings → Integrations). The form is generated from each integration's schema.
- Credentials of integrations are stored encrypted (AES-256-GCM with `LABDECK_SECRET_KEY`) and are
  never returned by the API; leaving a field empty keeps the stored value. Alternatively, a field
  can reference an environment variable as `${SECRET_NAME}` (prefix `SECRET_` only).
- "Test connection" before saving, with clear error messages (wrong key, timeout, DNS, refused
  connection, self-signed certificate, invalid JSON).
- Per integration: polling interval (15 s – 24 h), timeout, TLS certificate verification, active.
  Results, status changes and metrics (raw and daily values) are stored; old data is removed
  according to the retention settings.
- Healthchecks.io (cloud or self-hosted, read-only API key is enough): checks, backup tag, counts.
- Dashboard: overall status ("All good" / "2 problems") based on the integrations.

### Changed
- GitLab CI: Go and web tests on every push and merge request; image build check on other
  branches; image push to the GitLab registry on `main` (`:main`) and for version tags
  (`:X.Y.Z`, `:X.Y`, `:latest`).

## [0.1.0] – 2026-10-06

### Added
- First version: a single container with Go server, SQLite database and embedded web interface.
- Sign-in with one password (`APP_PASSWORD`, plain text or bcrypt hash), sessions bound to the
  password, login rate limit, protection against cross-site requests, `/healthz`.
- Live updates via Server-Sent Events.
- Web interface in German and English, light/dark/system theme, usable on phones, installable
  as an app (PWA).
- Dashboard with overall status; pages for backups, checks, services and history are prepared.
- Settings: display time zone, retention of raw data and daily values.
- `LABDECK_SECRET_KEY` (32 bytes, Base64) for encrypting the credentials of integrations;
  checked on startup.
- Docker image (distroless, non-root) and Docker Compose file.
