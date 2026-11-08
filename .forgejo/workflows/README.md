# Forgejo-Workflows

Dieses Verzeichnis existiert absichtlich, auch ohne Workflow-Dateien.

Forgejo Actions liest `.github/workflows` nur, wenn es **kein** `.forgejo/workflows` gibt.
Die Workflows in `.github/workflows` sind für GitHub gedacht (Image nach ghcr.io, GitHub-Release)
und laufen dort, nachdem der Push-Spiegel den Stand von Forgejo übertragen hat. Ohne dieses
Verzeichnis würde Forgejo sie zusätzlich selbst ausführen.

Eigene Forgejo-Workflows (`*.yml`) gehören hierher. Siehe [docs/forgejo-mirror.md](../../docs/forgejo-mirror.md).
