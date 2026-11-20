---
name: dashboard-design
description: Design and build dashboards, admin panels and internal tools that people can read at a glance - what the dashboard is for, navigation (sidebar or top bar), the few key numbers on top, a clear grid, filters and date ranges, detail pages, settings, and loading, empty and error states, on desktop, tablet and phone. Use before changing labdeck's dashboard, tiles, grid, page layout, detail pages or settings UI.
---

# Dashboard-Design (labdeck)

Ziel jeder Seite in labdeck: **in 5 Sekunden wissen, ob alles in Ordnung ist – und wenn nicht, wo.**
Erst die Frage klären, dann gestalten. Gilt zusammen mit CLAUDE.md (Design 1:1 wie dnsdeck).

## 1. Zweck vor Layout
- Pro Seite eine Leitfrage aufschreiben (Dashboard: „Ist alles ok?“, Backups: „Welches Backup ist alt/kaputt?“,
  Systemdetail: „Was ist mit diesem System los?“). Alles, was die Frage nicht beantwortet, kommt tiefer oder weg.
- Rangfolge auf dem Dashboard, von oben nach unten:
  1. **Gesamtstatus** (eine Zeile, Ampel, Anzahl Probleme)
  2. **Probleme** als Zeilen (was, wo, Wert, seit wann, Link zur Stelle)
  3. **Kernzahlen** – wenige kompakte Status-Kacheln in einer Reihe
  4. **Breite Inhalte** – Verläufe, Host-Listen, Zeitleisten
  5. **Ereignisse** ganz unten
- Ruhe ist Information: Kacheln ohne Problem sind neutral (kleiner grüner Punkt), Farbe/Leiste nur bei Problemen.

## 2. Seite und Raster
- Inhaltsbreite: auf großen Bildschirmen nutzen (Richtwert `max-w-[1600px]`), **immer mittig** (`mx-auto`).
  Kein Inhalt, der an einer Seite klebt; nie waagerechtes Scrollen der Seite.
- Dashboard-Raster: 12 Spalten (react-grid-layout, `DashboardGrid.tsx`). Breakpoints:
  Desktop 12 Spalten, Tablet 6, Handy 1 Spalte untereinander in Prioritätsreihenfolge.
- **Größen nach Inhaltstyp** (w × h in Rasterzeilen, Werte an ROW_HEIGHT anpassen):
  | Inhalt | Größe |
  |---|---|
  | Status-Kachel (Zahl + 1–2 Zeilen) | 3–4 × kompakt (so hoch wie der Inhalt, kein Leerraum) |
  | Kachel mit Sparkline | 4 × etwas höher |
  | Diagramm / Verlauf | 6–8 × mittel |
  | Liste/Tabelle (Hosts, Zeitleiste) | 12 (oder 8) × nach Zeilenzahl |
  | Ereignisse | 12 × groß |
- Kacheln einer Reihe haben **gleiche Höhe**; Reihen sind voll (Breiten summieren sich auf 12). Keine Löcher:
  neue Kacheln in die passende Zone einsortieren (Status-Reihe vs. breite Inhalte), nicht nur „höchste freie Stelle“.
- Faustregel Leerraum: Ist eine Kachel zu mehr als ~30 % leer, ist sie zu groß oder zeigt zu wenig.
- Abstände einheitlich (`gap-4`/GRID_MARGIN), innen `p-5`/`px-6` wie bestehende Cards.

## 3. Anatomie einer Kachel
Wie dnsdeck: Icon-Box links, Titel, Status (Punkt oder Badge „n Probleme“), Chevron rechts → Detailseite.
- **Eine** Hauptzahl groß (`text-2xl`/`text-3xl`, `tabular-nums`), Einheit klein daneben.
- Darunter höchstens 1–2 Sekundärzeilen in `text-muted-foreground`.
- Optional Sparkline/Mini-Diagramm am unteren Rand, füllt die Restfläche statt Leerraum.
- Ganze Kachel klickbar zur genauen Stelle (`/systeme/:id`, `#res-…` usw.), Fokusring sichtbar.
- Titel kürzen mit `truncate` + `title`-Attribut; nie Layout sprengen.

## 4. Navigation und Detailseiten
- Feste Sidebar (Desktop), Menü auf dem Handy; aktive Seite hervorgehoben; Fehlerzahl an „Dashboard“.
- **Jede Information hat genau einen Ort** (CLAUDE.md): Kachel fasst zusammen, Detailseite zeigt alles.
- Detailseite: Kopf (Name, Status, Typ · Version · letzte Abfrage, Aktionen) → Probleme → Kennzahlen → Inhalte.
- Sprungziele per `data-anchor` + `useHashTarget`; Filter, die das Ziel verstecken würden, beim Sprung lockern.

## 5. Filter und Zeiträume
- Zeitraum als `Segmented` (1 h · 24 h · 7 Tage · 30 Tage), sinnvoller Standard, nah am Diagramm.
- Filter nur, wenn die Liste > ~10 Einträge hat; Suchfeld + Zustand + Sortierung in einer Zeile, auf dem Handy umbrechen.
- Standard-Sortierung: **Probleme zuerst**, dann Name (numerisch).

## 6. Zustände (immer alle vier bauen)
- **Laden:** `Skeleton` in exakt der späteren Größe – keine Layout-Sprünge.
- **Leer:** Icon, ein Satz Erklärung, ein Link/Knopf zum nächsten Schritt (z. B. „Integration hinzufügen“).
- **Fehler:** was ist kaputt + seit wann; letzte gültige Werte weiter zeigen, als „veraltet seit …“ markiert.
- **Teilweise:** einzelne Quelle fehlt → nur diese Kachel markieren, Rest normal.

## 7. Einstellungen
- Gruppen als Cards mit Überschrift + Unterzeile; Formulare aus `Schema()`; Hilfetexte unter dem Feld.
- Gefährliche Aktionen (Löschen) mit Bestätigungsdialog, Ladezustand und sichtbarem Fehler.

## 8. labdeck-Bausteine (wiederverwenden statt neu bauen)
`Card`, `StatusBadge`, `StatusTile`, `UptimeBar`, `Meter`, `Sparkline`, `MetricChart`, `MetricTile`, `Segmented`,
`Skeleton`, `PageHeader`, `Section`, `ProblemList`/`ProblemRows`, `LiveRow`. Farben nur über Tokens
(`--success`, `--warning`, `--destructive`, `--muted-foreground`, `--chart-1…3`). Alle Texte in
`web/src/locales/{de,en}.json`.

## 9. Prüfen, bevor es fertig ist
1. Screenshots mit Playwright bei **1920**, **1280** und **390** px Breite, jeweils hell und dunkel
   (`e2e/screenshots.mjs` bzw. ein kurzes Skript; Ergebnis mit dem Read-Tool ansehen).
2. Checkliste: Gesamtstatus oben? Kein Loch im Raster? Keine Kachel > 30 % leer? Gleiche Höhen je Reihe?
   Inhalt mittig, kein Querscrollen? Problemfarbe nur bei Problemen? Handy: sinnvolle Reihenfolge?
3. `npm run lint`, `npm run build`, betroffene E2E-Tests; E2E-Tests dürfen nicht vom Layout früherer Tests abhängen.
