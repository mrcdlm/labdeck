# Changelog

All notable changes to this project are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
