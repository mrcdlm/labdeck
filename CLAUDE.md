# CLAUDE.md – Projekt „labdeck“ (Arbeitstitel)

Selbst gehostetes HomeLab-Dashboard: zeigt Backups, Healthchecks, Kennzahlen aus Diensten wie Immich
und beliebige weitere Werte über Standard-Schnittstellen – alles im Web-UI konfigurierbar.
Läuft als **ein einziger Docker-Container** auf jedem Linux-Host mit Docker Compose (Referenzsystem: openSUSE MicroOS).

Schwesterprojekt und **Design-Vorlage: [dnsdeck](https://github.com/mrcdlm/dnsdeck)**. Stack, Aufbau, UI-Komponenten,
Farben, Sicherheitskonzept und Arbeitsweise werden von dort übernommen (gleiche Lizenz, gleicher Autor).
Wo diese Datei nichts anderes sagt, gilt: so machen wie in dnsdeck.

## Ziele
1. Auf einen Blick sehen, ob im HomeLab alles in Ordnung ist (eine Ampel pro Bereich, Probleme ganz oben).
2. Backups überwachen: wann lief welches Backup zuletzt, war es erfolgreich, ist es überfällig?
3. Healthchecks.io-Checks (Cloud oder self-hosted) anzeigen inkl. Uptime-Verlauf.
4. Kennzahlen aus Diensten anzeigen, z. B. Anzahl Fotos/Videos und Speicherbelegung in Immich.
5. **Alles im Web-UI einstellbar:** Integrationen anlegen, testen, bearbeiten; Dashboard-Kacheln anordnen; Schwellwerte; Webhooks.
6. Neue Datenquellen möglichst **ohne neuen Code** einbinden (generische HTTP-JSON- und Prometheus-Quellen).

## Tech-Stack (verbindlich, identisch zu dnsdeck)
- **Backend:** Go (aktuelle stabile Version), `net/http` + `chi`
- **Datenbank:** SQLite (`modernc.org/sqlite`, CGO-frei) – Datei unter `/data/app.db`, Migrationen beim Start
- **Frontend:** React + TypeScript + Vite, Tailwind CSS v4, shadcn/ui, TanStack Query, lucide-react, Recharts, react-router, i18next (DE/EN)
- **Auslieferung:** Frontend per `go:embed` im Go-Binary → ein Binary, ein Container
- **Live-Updates:** Server-Sent Events (`/api/events`)
- **Container:** Multi-Stage-Build, `gcr.io/distroless/static`, non-root (UID 65532), Ziel < 30 MB, amd64 + arm64
- **CI:** GitHub Actions → Multi-Arch-Image nach `ghcr.io`
- **Lint/Checks:** `go vet`, `go test ./...`, `oxlint`, Locale-Prüfskript, Playwright-E2E

## Design (1:1 wie dnsdeck)
- `web/src/index.css` aus dnsdeck übernehmen: shadcn-Tokens in OKLCH (Zinc-Grautöne), `--radius: 0.65rem`,
  zusätzliche Tokens `--success` und `--warning`, Dark-Mode über `.dark`-Klasse
- Theme-Umschalter **System / Hell / Dunkel** (System = Standard, folgt live dem Betriebssystem), Sprache **DE | EN** in der Sidebar
- Layout: feste linke Sidebar (Logo oben, Navigation mit lucide-Icons, unten „Live“-Punkt, Sprache, Theme, Abmelden),
  Inhalt rechts mit Seitentitel + grauer Unterzeile
- Karten (`Card`) mit dezentem Rahmen, große Kennzahlen in Tabular-Nums, Status als farbige Badges
  (grün = ok, gelb = Warnung, rot = Fehler, grau = unbekannt/pausiert) – Komponente `StatusBadge` aus dnsdeck übernehmen
- Übersichtskacheln wie die dnsdeck-Dashboard-Kacheln: Icon-Box links, Titel, Zahl + Badges, Chevron rechts → Detailseite
- Uptime-Balken (`UptimeBar`) aus dnsdeck für Healthchecks- und Backup-Verlauf wiederverwenden
- Mobil voll nutzbar (Sidebar wird zum Menü), Skeletons beim Laden, keine Layout-Sprünge
- Neue Diagrammfarben nur mit validierter Palette (Kontrast + Farbsehschwächen), wie in dnsdeck kommentiert

## Verzeichnisstruktur
```
/cmd/server              main.go
/cmd/mock                Mock-Server für Healthchecks, Immich, PBS, Prometheus (Entwicklung + Tests)
/internal/config         Env-Variablen + Settings aus DB
/internal/integrations   Integration-Interface, Registry, Konfig-Schema
/internal/integrations/healthchecks
/internal/integrations/immich
/internal/integrations/pbs          Proxmox Backup Server
/internal/integrations/httpjson     generische JSON-Quelle
/internal/integrations/prometheus   generische PromQL-Quelle
/internal/integrations/snmp         Switches per SNMP (IF-MIB, ENTITY-MIB), Frontbild
/internal/integrations/redfish      Server-Hardware per DMTF Redfish (iLO, iDRAC …)
/internal/integrations/pihole       Pi-hole v6 (REST-API mit Session)
/internal/live           Ringpuffer: Kennzahlen der letzten Stunde in Abfrageauflösung
/internal/backups        Backup-Auswertung (Frische, Erfolg, Überfälligkeit) über alle Quellen
/internal/secrets        Verschlüsselte Ablage von Zugangsdaten
/internal/notify         generische Webhooks (aus dnsdeck übernehmen)
/internal/i18n           Nachrichtenkatalog DE/EN
/internal/scheduler      Polling je Integration
/internal/store          SQLite, Migrationen
/internal/api            HTTP-Handler, SSE
/web                     React-Frontend, Texte in web/src/locales/{de,en}.json
/deploy                  docker-compose.yml, .env.example
```

## Integrations-Architektur („Standard-Schnittstellen“)
Jede Datenquelle ist eine **Integration** mit einheitlichem Go-Interface:

```go
type Integration interface {
    Type() string                                     // "healthchecks", "immich", "pbs", "httpjson", "prometheus"
    Schema() ConfigSchema                             // Felder für das UI-Formular (Name, Typ, Pflicht, Secret?, Default, Hilfetext DE/EN) + Category für „Systeme“
    Test(ctx context.Context, cfg Config) error       // „Verbindung testen“-Button
    Collect(ctx context.Context, cfg Config) (Snapshot, error)
}
```

- **Das Frontend erzeugt die Einstellungsformulare aus `Schema()`** – für eine neue Integration ist kein Formular-Code nötig.
- `Snapshot` enthält normierte Daten: `Metrics` (Schlüssel, Wert, Einheit, Status), `Checks` (Name, Status, letzter/nächster Zeitpunkt),
  `Backups` (Quelle, Ziel/Gruppe, letzter Lauf, Ergebnis, Größe, Dauer). Darauf bauen Dashboard, Backups-Seite und Benachrichtigungen auf.
- Mehrere Instanzen pro Typ möglich (z. B. zwei Healthchecks-Projekte, zwei PBS).
- Pro Instanz: Polling-Intervall (Default 60 s, min. 5 s – Rohwerte landen trotzdem höchstens je Minute in der DB, die letzte Stunde hält `internal/live`), Timeout, TLS prüfen ja/nein (für selbst signierte Zertifikate im LAN), aktiv/inaktiv.
- Ergebnisse und Statuswechsel landen in der DB → Verlauf und Uptime. Rohdaten 30 Tage, Tageswerte 1 Jahr (für Diagramme wie „Fotos pro Monat“).
- Externe APIs in Tests immer gegen `/cmd/mock` bzw. `httptest` – nie echte Dienste.

### Healthchecks.io (Cloud und self-hosted)
- Basis-URL einstellbar (Default `https://healthchecks.io`), Header `X-Api-Key` – **Read-only-Key genügt**
- `GET /api/v3/checks/` → Name, Slug, Tags, Status (`new`, `up`, `grace`, `down`, `paused`), `last_ping`, `next_ping`, Zeitplan
- `GET /api/v3/checks/{uuid}/flips/` → Statuswechsel für Uptime-Balken 24 h / 7 Tage / 30 Tage
- Checks mit einstellbarem Tag (Default `backup`) werden **zusätzlich als Backup** gewertet und erscheinen auf der Backups-Seite
- Filter nach Tags im UI

### Immich
- Basis-URL + API-Key (Header `x-api-key`); benötigte Berechtigung: `server.statistics` (im UI-Hilfetext erwähnen)
- `GET /api/server/statistics` → Fotos, Videos, Speicherbelegung, Aufschlüsselung pro Benutzer
- `GET /api/server/about` bzw. `/api/server/version` → Version (optional, falls Berechtigung vorhanden)
- Vor der Umsetzung Endpunkte und Felder gegen die aktuelle Immich-API-Doku prüfen (die API ändert sich zwischen Versionen)
- Anzeige: Kachel „Immich – 48.213 Fotos · 1.204 Videos · 412 GB“, Detailkarte pro Benutzer, Verlauf der Anzahl als Diagramm

### Proxmox Backup Server (Backups)
- Basis-URL (Port 8007), API-Token (`Authorization: PBSAPIToken=user@realm!tokenid:secret`), Rolle `DatastoreAudit` genügt
- Datastores: Belegung/Kapazität; je Backup-Gruppe (VM/CT/Host): letzter Snapshot, Größe, Verify-Status
- Endpunkte gegen die aktuelle PBS-API-Doku verifizieren

### Generisch: HTTP-JSON
- URL, Methode, Header (Secrets erlaubt), Ausdruck zum Auslesen (z. B. `gjson`-Pfad), Einheit, Anzeigename
- Optional Schwellwerte für Warnung/Fehler, optional „Wert ist ein Zeitstempel → Alter berechnen“ (damit lässt sich jedes Backup-Tool mit Status-JSON anbinden)
- Damit z. B. Uptime Kuma, Paperless, Nextcloud, Home Assistant ohne eigenen Code

### Generisch: Prometheus
- Basis-URL, optional Basic Auth / Bearer, beliebige PromQL-Abfragen (`/api/v1/query`), je Abfrage Anzeigename, Einheit, Schwellwerte
- Deckt alles ab, was Exporter liefert (node_exporter, restic-/borg-Exporter, SMART, …)

### Grundsatz: Standard-Schnittstellen statt Geräte-Code
Neue Quellen sprechen offene Protokolle; Herstellerspezifisches nur als optionale Ergänzung.
- **SNMP** (`snmp`): v2c/v3, nur RFC-MIBs (SNMPv2-MIB, IF-MIB, ENTITY-MIB). Referenz Cisco C1300, jeder Switch passt.
  Frontbild (RJ45/SFP, Einheit, Position) aus ENTITY-MIB, sonst aus Portnamen/Modellname; Feld *Frontlayout* überschreibt.
  Ports: Ressourcen `kind=port` unter `kind=switch`; Raten aus 64-Bit-Zählern (Zustand im Speicher).
- **Redfish** (`redfish`): DMTF-Felder, Pfade nur aus `@odata.id` (keine festen IDs). Referenz HPE iLO 5 mit
  `Oem.Hpe.AggregateHealthStatus` und SmartStorage; iDRAC, XCC, Supermicro über dieselben DMTF-Felder.
  Ressource `kind=server` (iLO liefert keine CPU-/RAM-Auslastung → kein „Host“).
- **Pi-hole** hat keine Standard-API → eigene Integration für v6; Session wiederverwenden (Pi-hole begrenzt Sessions).

## Backups-Auswertung
- Quellen: Healthchecks-Checks mit Backup-Tag, PBS-Gruppen, HTTP-JSON-Zeitstempel
- Je Backup im UI einstellbar: **maximales Alter** (Default 26 h), ab dem es als überfällig gilt; Warnung ab 80 % davon
- Status: ok / Warnung (bald fällig) / überfällig / fehlgeschlagen / unbekannt
- Backups-Seite: Tabelle mit Quelle, Name, letzter Lauf („vor 3 h“), Ergebnis, Größe, Verlauf als UptimeBar; sortiert nach Problemen zuerst

## UI-Seiten
- **Dashboard:** oben Gesamtstatus („Alles in Ordnung“ / „2 Probleme“) und darunter **jedes Problem als eigene Zeile**
  (was, wo, Wert, seit wann; Link zur genauen Stelle per `#backup-…`, `#check-…`, `#res-…`, `#grp-…` = `data-anchor`).
  Kacheln zeigen nur die Anzahl ihrer Probleme, farbige Leiste nur bei Problemen. Darunter frei anordenbare Kacheln
  (Backups, Healthchecks, Immich, eigene Kennzahlen), unten letzte Ereignisse.
  Mehrere Dashboards (Reiter, `/d/:id`), Raster aus 12 Spalten (x, y, w, h; react-grid-layout), Bearbeiten direkt auf
  dem Dashboard. Neue Kacheln kommen nur aufs Haupt-Dashboard (`main`). Kennzahl-Kacheln (`metric:<id>`) mit
  1–3 Kennzahlen als Zahl/Sparkline/Diagramm/Tacho, Zeitraum 1 h (live) bis 30 Tage. Reihenfarben `--chart-1…3`
  (validiert, siehe index.css). SSE trägt weiterhin nur Themen – Live-Werte kommen über die API.
- **Backups:** siehe oben
- **Checks:** alle Healthchecks-Checks, gruppiert nach Projekt/Tag, Uptime-Balken
- **Systeme** (`/systeme`): alle Integrationen nach `Category` aus dem Schema gruppiert, Probleme zuerst, Suche über
  Ressourcen. **Eine Detailseite je Integration** (`/systeme/:id`) mit allem: Probleme, Kennzahlen + Diagramm,
  Hosts/Gäste, Hardware, Switch-Front, Tabellen. Jede Information hat genau einen Ort – keine zweite Seite mit
  denselben Daten anlegen. `/dienste` und `/ressourcen` leiten weiter.
- **Verlauf:** Statuswechsel und Fehler aller Integrationen, filterbar
- **Einstellungen:**
  - *Integrationen:* hinzufügen (Typ wählen → Formular aus Schema), bearbeiten, „Verbindung testen“, aktivieren/deaktivieren, löschen
  - *Dashboard:* Dashboards anlegen, sortieren, löschen; Kacheln ein-/ausblenden (Anordnen und Größe auf dem Dashboard); gespeichert in der DB
  - *Backups:* Tag-Name für Healthchecks, Standard-Höchstalter
  - *Benachrichtigungen:* Webhooks wie in dnsdeck (Vorlagen für ntfy, Gotify, Discord, Telegram, Home Assistant),
    Ereignisse: Check down/wieder up, Backup überfällig/fehlgeschlagen, Integration nicht erreichbar, Schwellwert überschritten
  - *Allgemein:* Zeitzone für Anzeige, Aufbewahrungsdauer

## Konfiguration & Sicherheit
- Per Env-Variable nur das Nötigste: `APP_PASSWORD`, `LABDECK_SECRET_KEY` (32 Byte, Base64), `LABDECK_VERSION`, `LABDECK_PORT`, `TZ`, `LOG_LEVEL`
- **Zugangsdaten der Integrationen werden im UI eingegeben**, damit wirklich alles im Web-Frontend einstellbar ist:
  - in der DB **AES-256-GCM-verschlüsselt** mit `LABDECK_SECRET_KEY` gespeichert
  - Felder im Schema als `secret` markiert → API gibt sie **nie** zurück, nur „gesetzt / nicht gesetzt“; im Formular leer lassen = unverändert
  - alternativ Verweis auf Env-Variable `${SECRET_NAME}` (nur Präfix `SECRET_` auflösbar), wie die `WEBHOOK_`-Platzhalter in dnsdeck
  - Secrets nie in Logs, Fehlermeldungen oder Test-Ausgaben
- Ausgehende Anfragen nur an konfigurierte Integrationen (kein frei steuerbarer Proxy über die API)
- Login, Session-Cookie, Rate-Limit, `/healthz`, `Secure`-Cookie hinter TLS-Proxy: wie dnsdeck
- Kein Port ins Internet; Zugriff über Reverse Proxy mit TLS oder Cloudflare Tunnel + Access

## Arbeitsweise für Claude
- dnsdeck als Referenz klonen (`git clone https://github.com/mrcdlm/dnsdeck ../dnsdeck`) und Design, UI-Komponenten,
  Login, SSE, Webhooks, Dockerfile, CI und Mock-Ansatz von dort übernehmen statt neu zu erfinden
- In kleinen, lauffähigen Schritten arbeiten; nach jedem Meilenstein committen
- Go: `go vet` und `go test ./...` grün; jede Integration hat Tests gegen Mock-Antworten (inkl. Fehlerfälle: 401, Timeout, kaputtes JSON)
- Frontend: `npm run build` und `npm run lint` müssen durchlaufen; alle Texte in DE und EN
- Keine Secrets in Code, Tests oder Beispieldateien
- Bei Unklarheiten zuerst fragen, bevor Architektur geändert wird

## Meilensteine
1. **Grundgerüst:** Go-Server, SQLite + Migrationen, Login, Layout/Theme/i18n aus dnsdeck, leeres Dashboard, Dockerfile, Compose
2. **Integrations-Framework:** Interface, Registry, Schema-basierte Formulare, verschlüsselte Secrets, Scheduler, „Verbindung testen“, `/cmd/mock`
3. **Healthchecks.io + Backups-Seite:** Checks-Seite, Uptime-Balken, Backup-Tag, Höchstalter, Gesamtstatus auf dem Dashboard
4. **Immich + Dienste-Seite:** Statistik-Kachel, Verlauf als Diagramm
5. **Generische Quellen:** HTTP-JSON und Prometheus inkl. Schwellwerte
6. **Proxmox Backup Server**
7. **Dashboard-Editor + Benachrichtigungen + Verlauf-Seite**
8. **CI/CD:** GitHub Actions, Multi-Arch-Image auf ghcr.io, Release-Tags, README DE/EN mit Screenshots