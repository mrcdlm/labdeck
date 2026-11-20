---
name: data-display
description: Choose and build the right chart, table, number or list for each piece of data in a dashboard or report - KPI cards with honest comparisons, line, bar and stacked charts, tables with sorting, filtering and pagination, color that carries meaning, readable axes, tooltips and accessible alternatives. Use in labdeck whenever a metric, chart, table or list is added or changed. For palette validation also load the dataviz skill.
---

# Daten richtig darstellen (labdeck)

Erst fragen: **Welche Frage beantwortet diese Darstellung?** Dann die einfachste Form wählen, die sie beantwortet.
Für Farbpaletten und deren Prüfung zusätzlich den Skill `dataviz` laden.

## 1. Form wählen
| Frage | Form |
|---|---|
| Wie viel / wie ist es jetzt? | **Zahl** (KPI) – ggf. mit Vergleich |
| Wie voll? (Kapazität) | **Meter** mit Wert / Gesamt |
| Wie entwickelt es sich? | **Linie** (Sparkline in Kacheln, `MetricChart` auf Detailseiten) |
| Zustand über Zeit (ok/Fehler) | **UptimeBar** / Zeitleiste – keine Linie |
| Welche Kategorie ist größer? | **Balken**, sortiert, Nullbasis |
| Anteile am Ganzen (≤ 5 Teile) | **gestapelter Balken**; keine Torten mit vielen Stücken |
| Viele Elemente mit Eigenschaften | **Tabelle** |
| Rangfolge / Top-N | **Liste mit Inline-Balken** |
| Ja/Nein, Netz/Batterie | **Badge/Text**, kein Diagramm (`isStateUnit`) |

Keine 3D-, Radar- oder Doppelachsen-Diagramme. Höchstens 3 Reihen pro Diagramm (`--chart-1…3`).

## 2. KPI-Kacheln ehrlich
- Wert + Einheit + **Kontext**: gegen Kapazität („1,1 GB / 3 GB“), Schwellwert oder Vorperiode.
- Vergleiche nur über **gleich lange Zeiträume** (24 h vs. vorherige 24 h); absolut und relativ („+120 · +2 %“).
- Richtung ≠ Bewertung: mehr Fotos ist neutral, mehr Fehler schlecht – Farbe nur bei echter Bewertung.
- Keine Scheingenauigkeit: 2–3 signifikante Stellen („81,6 %“, „105 GB“), Rohwert im `title`.
- Fehlender Wert = „—“ mit Erklärung im Tooltip (warum fehlt er?), nie 0 vortäuschen.

## 3. Zahlen und Zeit formatieren
- `tabular-nums`, Locale aus i18n (de: „6.310“, „81,6 %“), Helfer aus `@/lib/format` (`formatNumber`, `formatBytes`, `formatUptime`).
- Relative Zeit („vor 3 h“, `relativeTime`) + absolute Zeit im `title` (`absoluteTime`).
- Einheiten konsistent je Kennzahl; Bytes binär oder dezimal – aber überall gleich.

## 4. Farbe trägt Bedeutung
- Statusfarben **nur** für Status: grün ok, gelb Warnung, rot Fehler, grau unbekannt/pausiert (`StatusBadge`).
- Farbe nie alleiniger Träger: zusätzlich Text, Icon oder Form (Badge-Text, Punkt + Label).
- Reihenfarben ausschließlich `--chart-1…3` (validiert, siehe `web/src/index.css`); Kontext/Vergleich in Grau.
- Neue Farben nur nach Prüfung (Kontrast, Farbsehschwächen) mit dem `dataviz`-Validator.

## 5. Achsen, Beschriftung, Tooltips
- Wenige Ticks (3–5), Einheit in der Achse oder im Titel, keine Gitternetz-Flut.
- Balken immer ab 0; Linien dürfen gezoomt sein, dann Achse deutlich beschriften.
- Eine Reihe → keine Legende; mehrere → direkt beschriften oder kompakte Legende über dem Diagramm.
- Tooltip: exakter Wert + Einheit + Zeitpunkt; per Tastatur erreichbar.
- Zu wenig Daten → eine Linie bzw. Hinweis „noch zu wenig Werte“, keine leere Fläche.

## 6. Tabellen
- Standard-Sortierung: Probleme zuerst, dann Name (numerisch: `localeCompare(…, { numeric: true })`).
- Zahlen rechtsbündig, Text linksbündig, Status als Badge in eigener Spalte.
- Sortierbare Spalten mit `aria-sort`; Filter/Suche über der Tabelle; ab ~50 Zeilen paginieren oder „mehr zeigen“.
- Kopfzeile bei langen Tabellen sticky; Zeilen klickbar → Detail/Verlauf (`LiveTableRow`).
- **Handy:** Tabelle (`hidden md:block`) wird zur Kartenliste (`md:hidden`, `LiveItem`) – Muster aus `ResourceParts.tsx`/`VpnView.tsx`.
- Beide Varianten tragen dieselben `data-anchor`-Werte.

## 7. Barrierefreiheit
- Diagramme mit `role="img"` + `aria-label` (Kurzfassung: „Fotos, 30 Tage, von 6.100 auf 6.310“).
- Meter mit `role="meter"`, `aria-valuenow/min/max`.
- Kontrast ≥ 4,5:1 für Text, ≥ 3:1 für Linien/Flächen; beide Themes prüfen.

## 8. Prüfen
Screenshot hell/dunkel ansehen; Fragen: Beantwortet die Form die Frage? Ist jede Farbe begründet?
Stimmen Einheiten und Rundung? Gibt es einen Leer- und Fehlerzustand? Texte in DE und EN?
