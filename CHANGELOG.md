# Changelog

All notable changes to this project are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Switch integration via SNMP v2c/v3 using standard MIBs (IF-MIB, ENTITY-MIB) – reference device
  Cisco C1300, works with any managed switch: link, speed, traffic rate, utilisation and errors
  per port; monitored ports (with a description or selected) that are down are problems. A
  schematic front panel (RJ45/SFP, stack units, LEDs, utilisation) on the resources page, the
  services page and the dashboard tile; port details with traffic history. The panel layout is
  derived from the device; "Front layout" in the form overrides it.
- Server management via DMTF Redfish – reference HPE iLO 5 (plus Oem.Hpe aggregate health and
  SmartStorage), also Dell iDRAC, Lenovo XCC, Supermicro, OpenBMC: health per component, power
  supplies, fans, temperatures with distance to the critical threshold, drives and logical
  drives with SSD remaining life, power draw, power state. Hardware cards on the resources page.
- Pi-hole v6 integration (app password, reuses its API session): queries, blocked %, active
  clients, blocklist size and age, blocking on/off, available updates.
- Several dashboards as tabs, each with a free 12-column grid: drag and resize tiles right on
  the dashboard (edit mode), add tiles from a dialog, rename, pick an icon, delete.
- Metric tiles: any metric of any integration (incl. switch ports and BMC sensors) as number,
  sparkline, chart with up to three series or gauge, for 1 hour (live) up to 30 days.
- Live values: polling interval from 5 s; the last hour is kept in memory at full resolution
  (history range "1h"), every chart ends at the current value.
- Proxmox VE integration (API token, role PVEAuditor): load of the nodes (CPU, RAM, root disk),
  status, CPU, RAM, disk and network per VM and container, storage usage, SMART state and
  remaining life of the disks, ZFS pool health, last backup per VM/container from the backup
  storages and the vzdump runs.
- Dockhand integration (API token, read permissions): CPU and RAM of every Docker host Dockhand
  manages, pending image updates, and status (incl. healthy / unhealthy), CPU, RAM and network
  per container.
- UPS via Network UPS Tools (upsd): mains or battery, charge, runtime, load, input voltage;
  running on battery and low runtime appear as problems.
- Resources page: one card per host/node with CPU, RAM and disk meters, below it the VMs and
  containers with search, state filter, sorting and a history chart per resource.
- Setting "Keep raw data per VM/container" (default 7 days) – with many guests this keeps the
  database small; daily values stay.
- Real, colored logos of the services (Healthchecks.io, Immich, Proxmox, Prometheus, NUT)
  and of the webhook presets, bundled offline.
- More motion: tiles and rows fade in, numbers count up and glide to new values, rows light up
  on live changes, the overall status pulses on errors and draws a check mark when all is good,
  uptime bars grow, meters fill, chart lines draw. Everything is off with "reduce motion".

- Dashboard tile "Hosts": all Proxmox VE nodes and Dockhand hosts with small CPU and RAM bars.
- Dashboard tile "Backup timeline": a history bar per backup (24 hours / 7 days), problems first.
- Small history charts (sparklines) in the service tiles, e.g. CPU over 24 hours or photos over
  30 days; hidden while there is too little data or the line is flat.

### Changed
- Minimum polling interval 5 s (was 15 s). Raw samples are stored at most once a minute per
  metric; daily values (min/max) still see every value.
- Settings → Dashboard manages the dashboards (order, delete, show/hide tiles); arranging and
  sizing happens on the dashboard itself. The old layout (sizes 1–3) is converted on first load;
  `/api/dashboard` stays as an alias of the main dashboard.
- "Verify TLS" is hidden for integrations that don't speak HTTP (SNMP, NUT).
- Dashboard is flatter: the tiles are the traffic light (colored bar on the left), each tile
  shows one large number, one secondary line and its own problems (two, more on click). The
  long problem list at the top is gone; the overall status is one line with counts of errors and
  warnings. Tiles with problems move to the front (can be turned off in Settings → Dashboard).
- Healthchecks.io as a traffic light: the Checks tile only shows green "All 7 checks OK" or red
  "2 checks down" with their names; failed checks are bundled into one problem row per project,
  checks in their grace period are no longer a problem; on the Services page Healthchecks is a
  compact status row instead of a card with counters and chart.
- Services page: grouped metrics are split into tables by kind (storage, disks, ZFS pools,
  filesystems, users); state values (mains/battery, OK/failed) get no chart.

### Fixed
- Dashboard could flicker endlessly when the page was about as tall as the window (the scrollbar
  appeared and disappeared, changing the grid width); numbers then stopped mid-count. The
  scrollbar space is now always reserved.
- Charts no longer restart their animation whenever a tile re-renders.
- `npm run screenshots` wrote the images outside the repository.

### Security
- Stored credentials are no longer sent to a new destination: after changing the address
  (scheme, host or port; host/port for SNMP and NUT) of an integration, or the URL of a webhook,
  stored secrets and tokens have to be entered again. This also applies to "Test connection".
  Previously, a session was enough to redirect stored credentials to another server.
- Built with Go 1.27.3 (fixes several vulnerabilities in `net/http`, HTTP/2, `crypto/tls` and
  `crypto/x509` of Go 1.27.1), `golang.org/x/crypto` 0.58.0; frontend dev dependencies updated
  (Vite, source-map-js).
- Additional response headers: `Cross-Origin-Opener-Policy`, `Cross-Origin-Resource-Policy`,
  `Permissions-Policy`, CSP `object-src 'none'`; request headers limited to 64 KB.
- New files in the data directory (database, WAL) are created readable by the container user
  only.
- Compose files: read-only root filesystem (`read_only`, `tmpfs` for `/tmp`) and `pids_limit`.

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
