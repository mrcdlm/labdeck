# Forgejo → GitHub (Push-Spiegel)

Entwickelt wird auf dem eigenen Forgejo (`origin`). GitHub ist ein **Push-Spiegel**: Forgejo
überträgt jeden Push (Branches und Tags) nach `github.com/mrcdlm/labdeck`. Dort laufen CI, der
Image-Build nach `ghcr.io` und die GitHub-Releases.

```
git push (Forgejo) ──► Push-Spiegel ──► GitHub ──► Actions: ci.yml, release.yml
```

| Push auf Forgejo | GitHub Actions                                                        |
|------------------|-----------------------------------------------------------------------|
| `main`           | CI (Go, Frontend, Playwright) · Image `ghcr.io/mrcdlm/labdeck:main`   |
| Tag `vX.Y.Z`     | Image `X.Y.Z`, `X.Y`, `latest` (amd64 + arm64) · GitHub-Release        |

## Einmalig einrichten

### 1. Token auf GitHub

GitHub → Settings → Developer settings → **Fine-grained personal access tokens** → *Generate new token*

- Repository access: *Only select repositories* → `mrcdlm/labdeck`
- Permissions → Repository:
  - **Contents: Read and write**
  - **Workflows: Read and write**: nötig, weil der Spiegel Dateien unter `.github/workflows`
    überträgt. Ohne diese Berechtigung lehnt GitHub jeden Push ab, der einen Workflow ändert.
  - Metadata: Read (wird automatisch gesetzt)
- Ablaufdatum notieren. Läuft das Token ab, schlägt die Synchronisation still fehl
  (Forgejo zeigt den Fehler in den Spiegel-Einstellungen).

### 2. Push-Spiegel in Forgejo

Forgejo → Repository `labdeck` → Einstellungen → Repository → **Spiegel-Einstellungen** → *Push-Spiegel hinzufügen*

- URL des entfernten Repositorys: `https://github.com/mrcdlm/labdeck.git`
- Benutzername: `mrcdlm`
- Passwort: das Token aus Schritt 1
- **Beim Pushen von Commits synchronisieren** aktivieren
- Intervall z. B. `8h` (Sicherheitsnetz, falls ein Sync-on-Push fehlschlägt)

Danach *Jetzt synchronisieren* klicken und auf GitHub prüfen, ob der Stand angekommen ist.

### 3. GitHub-Repository

- Settings → General: Beschreibung um „Mirror of …“ ergänzen (optional)
- Settings → Actions → General: Actions erlaubt lassen. Die Rechte (`packages: write`,
  `contents: write`) fordern die Workflows selbst an, die Standardeinstellung kann „Read“ bleiben.
- Keine Branch-Regeln, die Force-Pushes auf `main` oder Tags verbieten: Der Spiegel überträgt
  mit `git push --mirror` und muss GitHub exakt auf den Forgejo-Stand setzen dürfen.
- Dependabot: *Alerts* dürfen an sein, aber keine *version/security updates* (PR-Branches auf
  GitHub löscht der nächste Spiegel-Lauf wieder).

### 4. Forgejo Actions

Forgejo führt die Workflows aus `.github/workflows` **nicht** aus, weil es
`.forgejo/workflows/` gibt (siehe README dort). Wer zusätzlich auf Forgejo prüfen will, legt dort
einen eigenen Workflow an, z. B. nur `go vet` und `go test`.

## Arbeitsweise

- **Nur auf Forgejo ändern.** Commits, Merges oder Web-Edits auf GitHub überschreibt der nächste
  Spiegel-Lauf.
- Pull Requests auf GitHub: Änderungen lokal übernehmen (`git fetch https://github.com/mrcdlm/labdeck pull/<n>/head`),
  auf Forgejo pushen, den PR mit Verweis auf den Commit schließen.
- Release:

  ```sh
  # CHANGELOG.md: Abschnitt [X.Y.Z] anlegen, committen
  git tag vX.Y.Z
  git push origin main vX.Y.Z      # origin = Forgejo, der Spiegel bringt beides zu GitHub
  ```

  Fehlt der Abschnitt im Changelog, bricht der Release-Job ab (das Image ist dann trotzdem gebaut).
- Release-Texte nachträglich ändern: GitHub → Actions → „Release-Notes aktualisieren“.
