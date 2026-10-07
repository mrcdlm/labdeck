# Changelog

All notable changes to this project are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.4.0] – 2026-10-07

### Added
- Immich integration: photos, videos and storage usage in total and per user (with quota),
  server version. Needs an admin API key with the `server.statistics` permission.
- Services page: one card per integration with its metrics, a chart of the selected metric
  (24 hours, 7 days, 30 days, 1 year) and a table per user/group.
- Dashboard: a tile per service, e.g. "Immich – 48,213 photos · 1,204 videos · 412 GB".
- HTTP JSON integration: read any values from a JSON API with GJSON paths (GET or POST, headers,
  an encrypted header for credentials), with unit and thresholds. A timestamp can be shown as its
  age and treated as a backup – so any backup tool with a status JSON can be monitored.
- Prometheus integration: any PromQL instant queries with unit and thresholds, basic auth or
  bearer token; one row per label value (e.g. per instance); timestamps (e.g. restic exporter)
  as backups.
- Threshold breaches appear in the overall status on the dashboard.
- Proxmox Backup Server integration (API token, role DatastoreAudit): last snapshot, size and
  verify state per VM, container and host (all namespaces), datastore usage with thresholds.
  A failed verify marks the backup as failed.
- Backups without a history from their source get one from their runs (e.g. PBS snapshots):
  OK after a run, due soon from 80 % of the maximum age, then overdue until the next run.
- Notifications via webhooks (as in dnsdeck) with presets for ntfy, Gotify, Discord, Slack,
  Telegram and Home Assistant, live preview and test message. Each webhook has an encrypted token
  field, used as `${TOKEN}`; plain-text secrets are rejected. Events: check down / up again,
  backup overdue / failed / OK again, integration unreachable / reachable again, threshold
  exceeded. Language of the notifications is configurable.
- Backups are evaluated every minute and after every poll, so "overdue" is detected without new
  data and reported once.
- History page: status changes and errors of all integrations, filter by integration, kind and
  problems only, load older entries.
- Dashboard editor (Settings → Dashboard): show/hide tiles, order by drag and drop or arrows,
  width 1–3 columns; stored in the database. New tile "Recent events".
- GitHub Actions (as in dnsdeck): CI with Go, frontend and browser tests; multi-arch image
  (amd64, arm64) on ghcr.io for `main` and version tags, GitHub release with notes from this
  changelog.
- README in English and German with screenshots, SECURITY.md.

### Changed
- Addresses of integrations are stored as entered (a trailing slash is kept, some APIs need it).

## [0.3.0] – 2026-10-07

### Added
- Checks page: all Healthchecks.io checks grouped by project or tag, tag filter, last/next ping and
  uptime bars for 24 hours, 7 days and 30 days (from the status changes in Healthchecks.io).
- Backups page: every backup with status (OK, due soon, overdue, failed, never ran, paused), last
  run, when it is due, size and history; problems first. Checks with the backup tag appear
  automatically.
- Maximum age per backup (from the Backups page) and a default maximum age in Settings → Backups
  (26 hours); a warning starts at 80 % of it.
- Dashboard: overall status now lists overdue and failed backups and checks that are down or in
  their grace period, with tiles for backups and checks.

### Changed
- GitLab CI: every push runs the Go and web tests; images are only built for version tags
  (`:X.Y.Z` and `:X.Y`, no `:main` or `:latest`).

### Fixed
- GitLab CI: the image is built and pushed with BuildKit without attestations, so the GitLab
  registry accepts the manifest.

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
