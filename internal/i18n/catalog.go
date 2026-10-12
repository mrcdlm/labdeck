package i18n

type entry struct{ de, en string }

// catalog enthält alle Texte, die der Server ausliefert. Platzhalter: {name}.
var catalog = map[string]entry{
	"raw": {"{text}", "{text}"},

	// Allgemein / API
	"api.not_found":            {"nicht gefunden", "not found"},
	"api.bad_request":          {"ungültige Anfrage", "invalid request"},
	"api.internal":             {"interner Fehler", "internal error"},
	"api.unauthorized":         {"nicht angemeldet", "not signed in"},
	"api.live_unavailable":     {"Live-Updates nicht verfügbar", "Live updates are not available"},
	"api.settings_unavailable": {"Einstellungen nicht verfügbar", "Settings are not available"},
	"auth.wrong_password":      {"Passwort falsch", "Wrong password"},
	"auth.too_many_attempts":   {"Zu viele Fehlversuche, bitte kurz warten", "Too many failed attempts, please wait a moment"},

	// Geheimnisse
	"secret.no_key":         {"LABDECK_SECRET_KEY ist nicht gesetzt – Zugangsdaten können nur als ${SECRET_NAME} hinterlegt werden", "LABDECK_SECRET_KEY is not set – credentials can only be entered as ${SECRET_NAME}"},
	"secret.decrypt_failed": {"Gespeicherte Zugangsdaten lassen sich nicht entschlüsseln (LABDECK_SECRET_KEY geändert?) – bitte neu eingeben", "Stored credentials cannot be decrypted (LABDECK_SECRET_KEY changed?) – please enter them again"},
	"secret.env_prefix":     {"nur Env-Variablen mit Präfix {prefix} sind erlaubt (nicht {name})", "only environment variables with the prefix {prefix} are allowed (not {name})"},
	"secret.env_missing":    {"Env-Variable {name} ist nicht gesetzt", "environment variable {name} is not set"},

	// Integrationen: Formular
	"integration.field_unknown":      {"unbekanntes Feld {field}", "unknown field {field}"},
	"integration.field_required":     {"{field}: Pflichtfeld", "{field}: required"},
	"integration.field_type":         {"{field}: ungültiger Wert", "{field}: invalid value"},
	"integration.field_too_long":     {"{field}: zu lang", "{field}: too long"},
	"integration.field_option":       {"{field}: ungültige Auswahl", "{field}: invalid choice"},
	"integration.field_newline":      {"{field}: darf keinen Zeilenumbruch enthalten", "{field}: must not contain a line break"},
	"integration.field_range":        {"{field}: muss zwischen {min} und {max} liegen", "{field}: must be between {min} and {max}"},
	"integration.field_list_max":     {"{field}: höchstens {max} Einträge", "{field}: at most {max} entries"},
	"integration.field_item":         {"{field} {n}: {detail}", "{field} {n}: {detail}"},
	"integration.field_url":          {"{field}: bitte eine http(s)-Adresse angeben, z. B. https://dienst.example.com", "{field}: please enter an http(s) address, e.g. https://service.example.com"},
	"integration.field_url_userinfo": {"{field}: Benutzer/Passwort gehören nicht in die Adresse, sondern in die Felder für Zugangsdaten", "{field}: user name/password do not belong in the address but in the credential fields"},
	"integration.field_url_invalid":  {"ungültige Adresse", "invalid address"},
	"integration.field_secret_ref":   {"{field}: {detail}", "{field}: {detail}"},
	"integration.type_unknown":       {"unbekannter Integrationstyp {type}", "unknown integration type {type}"},
	"integration.name_invalid":       {"Name fehlt oder ist zu lang (max. 100 Zeichen)", "Name is missing or too long (max. 100 characters)"},
	"integration.interval_range":     {"Abfrageintervall muss zwischen {min} und {max} Sekunden liegen", "The polling interval must be between {min} and {max} seconds"},
	"integration.timeout_range":      {"Timeout muss zwischen {min} und {max} Sekunden liegen und kürzer als das Intervall sein", "The timeout must be between {min} and {max} seconds and shorter than the interval"},
	"integration.type_change":        {"Der Typ einer Integration kann nicht geändert werden", "The type of an integration cannot be changed"},
	"integration.not_found_id":       {"Integration nicht gefunden", "Integration not found"},
	"integration.no_data_yet":        {"Noch nicht abgefragt", "Not polled yet"},

	// Integrationen: Abfrage
	"integration.unauthorized":  {"Zugriff verweigert (HTTP {status}) – API-Key bzw. Token und Berechtigungen prüfen", "Access denied (HTTP {status}) – check the API key or token and its permissions"},
	"integration.not_found":     {"Adresse nicht gefunden (HTTP 404) – Basis-URL prüfen", "Address not found (HTTP 404) – check the base URL"},
	"integration.rate_limited":  {"Zu viele Anfragen (HTTP 429) – Abfrageintervall erhöhen", "Too many requests (HTTP 429) – increase the polling interval"},
	"integration.redirect":      {"Unerwartete Weiterleitung (HTTP {status}) – Basis-URL prüfen", "Unexpected redirect (HTTP {status}) – check the base URL"},
	"integration.redirect_host": {"Weiterleitung auf einen anderen Host wird aus Sicherheitsgründen nicht verfolgt – Basis-URL prüfen", "Redirects to another host are not followed for security reasons – check the base URL"},
	"integration.http":          {"Dienst antwortet mit HTTP {status}", "Service responded with HTTP {status}"},
	"integration.timeout":       {"Zeitüberschreitung – keine Antwort", "Timed out – no response"},
	"integration.canceled":      {"Abfrage abgebrochen", "Request canceled"},
	"integration.dns":           {"{host} lässt sich nicht auflösen (DNS des Containers prüfen)", "{host} does not resolve (check the container's DNS)"},
	"integration.refused":       {"Verbindung abgelehnt – Adresse und Port prüfen", "Connection refused – check address and port"},
	"integration.tls_untrusted": {"Zertifikat von unbekannter Stelle (selbst signiert?) – „TLS-Zertifikat prüfen“ ausschalten oder ein gültiges Zertifikat verwenden", "Certificate from an unknown authority (self-signed?) – turn off \"Verify TLS certificate\" or use a valid certificate"},
	"integration.tls_hostname":  {"Zertifikat gilt nicht für {host}", "Certificate is not valid for {host}"},
	"integration.tls_invalid":   {"Zertifikat ungültig: {detail}", "Invalid certificate: {detail}"},
	"integration.tls_not_https": {"Der Dienst spricht kein HTTPS – Adresse mit http:// angeben", "The service does not speak HTTPS – use an http:// address"},
	"integration.unreachable":   {"Nicht erreichbar: {detail}", "Unreachable: {detail}"},
	"integration.too_large":     {"Antwort zu groß", "Response too large"},
	"integration.bad_json":      {"Antwort ist kein gültiges JSON ({detail})", "Response is not valid JSON ({detail})"},

	// Healthchecks.io
	"healthchecks.unexpected": {"Antwort enthält keine Checks – ist die Adresse eine Healthchecks-Instanz?", "The response contains no checks – is the address a Healthchecks instance?"},

	// Backups
	"backup.not_found": {"Backup nicht gefunden", "Backup not found"},

	// Immich
	"immich.forbidden":  {"Zugriff verweigert (HTTP 403) – der API-Key muss zu einem Admin gehören und die Berechtigung server.statistics haben", "Access denied (HTTP 403) – the API key must belong to an admin and have the server.statistics permission"},
	"immich.unexpected": {"Antwort enthält keine Statistik – ist die Adresse eine Immich-Instanz?", "The response contains no statistics – is the address an Immich instance?"},

	// HTTP-JSON
	"httpjson.header_invalid":      {"Header Zeile {line}: bitte im Format Name: Wert", "Header line {line}: please use the format Name: value"},
	"httpjson.auth_header_invalid": {"Header mit Zugangsdaten: bitte im Format Name: Wert", "Header with credentials: please use the format Name: value"},
	"httpjson.path_missing":        {"{name}: Pfad {path} nicht in der Antwort gefunden", "{name}: path {path} not found in the response"},
	"httpjson.not_number":          {"{name}: {value} ist keine Zahl", "{name}: {value} is not a number"},
	"httpjson.not_timestamp":       {"{name}: {value} ist kein Zeitstempel (Unix-Zeit oder ISO 8601 erwartet)", "{name}: {value} is not a timestamp (Unix time or ISO 8601 expected)"},

	// Prometheus
	"prometheus.query_error": {"{name}: Abfrage ungültig – {detail}", "{name}: invalid query – {detail}"},
	"prometheus.matrix":      {"{name}: Abfrage liefert einen Bereich (z. B. [5m]) – bitte eine Funktion wie rate() oder last_over_time() verwenden", "{name}: the query returns a range (e.g. [5m]) – please use a function like rate() or last_over_time()"},
	"prometheus.unexpected":  {"Antwort ist kein Prometheus-Ergebnis – ist die Adresse Prometheus?", "The response is not a Prometheus result – is the address Prometheus?"},

	// Proxmox Backup Server
	"pbs.unexpected":        {"Antwort ist keine PBS-API – Adresse mit Port 8007 prüfen", "The response is not the PBS API – check the address with port 8007"},
	"pbs.no_datastores":     {"Das Token sieht keine Datastores – Rolle DatastoreAudit auf /datastore für das Token vergeben", "The token sees no datastores – give the token the role DatastoreAudit on /datastore"},
	"pbs.datastore_missing": {"Datastore {store} nicht gefunden oder für das Token nicht sichtbar", "Datastore {store} not found or not visible to the token"},

	// Webhooks (aus dnsdeck)
	"webhook.not_found":        {"Webhook nicht gefunden", "Webhook not found"},
	"webhook.unavailable":      {"Webhooks nicht verfügbar", "Webhooks are not available"},
	"webhook.env_prefix":       {"nur Env-Variablen mit Präfix {prefix} sind erlaubt (nicht {name})", "only environment variables with the prefix {prefix} are allowed (not {name})"},
	"webhook.env_missing":      {"Env-Variable {name} ist nicht gesetzt", "environment variable {name} is not set"},
	"webhook.token_missing":    {"${TOKEN} wird verwendet, aber im Feld Token ist nichts gespeichert", "${TOKEN} is used, but nothing is stored in the token field"},
	"webhook.in_url":           {"URL: {detail}", "URL: {detail}"},
	"webhook.in_header":        {"Header {name}: {detail}", "Header {name}: {detail}"},
	"webhook.template":         {"Template: {detail}", "Template: {detail}"},
	"webhook.template_json":    {"Template ergibt kein gültiges JSON – Texte mit {{json .Title}} einsetzen", "The template does not produce valid JSON – insert text with {{json .Title}}"},
	"webhook.name_invalid":     {"Name fehlt oder ist zu lang", "Name is missing or too long"},
	"webhook.method_invalid":   {"Methode muss POST, PUT, PATCH oder GET sein", "Method must be POST, PUT, PATCH or GET"},
	"webhook.url_missing":      {"URL fehlt", "URL is missing"},
	"webhook.url_scheme":       {"URL muss mit http:// oder https:// beginnen", "URL must start with http:// or https://"},
	"webhook.header_name":      {"ungültiger Header-Name {name}", "invalid header name {name}"},
	"webhook.header_forbidden": {"Header {name} ist nicht erlaubt", "Header {name} is not allowed"},
	"webhook.header_newline":   {"Header {name} enthält einen Zeilenumbruch", "Header {name} contains a line break"},
	"webhook.event_unknown":    {"unbekanntes Ereignis {name}", "unknown event {name}"},
	"webhook.secret_header":    {"Header {name} enthält vermutlich ein Geheimnis im Klartext – den Wert ins Feld Token eintragen und hier ${TOKEN} verwenden", "Header {name} probably contains a plain-text secret – put the value into the token field and use ${TOKEN} here"},
	"webhook.secret_userinfo":  {"URL enthält Benutzer/Passwort – den Wert ins Feld Token eintragen und ${TOKEN} verwenden", "The URL contains a user name/password – put the value into the token field and use ${TOKEN}"},
	"webhook.secret_query":     {"URL-Parameter {name} enthält vermutlich ein Geheimnis – den Wert ins Feld Token eintragen und ${TOKEN} verwenden", "URL parameter {name} probably contains a secret – put the value into the token field and use ${TOKEN}"},
	"webhook.secret_url":       {"Diese URL enthält selbst das Geheimnis (Token im Pfad) – die ganze URL ins Feld Token eintragen und als URL ${TOKEN} verwenden", "This URL itself is the secret (token in the path) – put the whole URL into the token field and use ${TOKEN} as URL"},
	"webhook.bad_url":          {"ungültige URL (nach Einsetzen der Platzhalter)", "invalid URL (after inserting the placeholders)"},
	"webhook.unreachable":      {"nicht erreichbar: {detail}", "unreachable: {detail}"},
	"webhook.redirect":         {"HTTP {status} – Weiterleitung auf einen anderen Host wird aus Sicherheitsgründen nicht verfolgt", "HTTP {status} – redirects to another host are not followed for security reasons"},
	"webhook.http":             {"HTTP {status}", "HTTP {status}"},
	"webhook.no_key":           {"LABDECK_SECRET_KEY ist nicht gesetzt – ein Token kann nicht gespeichert werden", "LABDECK_SECRET_KEY is not set – a token cannot be stored"},

	// Benachrichtigungen
	"notify.check_down.title":              {"Check down: {name}", "Check down: {name}"},
	"notify.check_down.message":            {"{integration} – letzter Ping {last}", "{integration} – last ping {last}"},
	"notify.check_up.title":                {"Check wieder up: {name}", "Check up again: {name}"},
	"notify.check_up.message":              {"{integration}", "{integration}"},
	"notify.backup_overdue.title":          {"Backup überfällig: {name}", "Backup overdue: {name}"},
	"notify.backup_overdue.message":        {"{integration} – letzter Lauf {last} (Höchstalter {max} h)", "{integration} – last run {last} (maximum age {max} h)"},
	"notify.backup_never":                  {"noch nie", "never"},
	"notify.backup_failed.title":           {"Backup fehlgeschlagen: {name}", "Backup failed: {name}"},
	"notify.backup_failed.message":         {"{integration} meldet einen Fehler.", "{integration} reports an error."},
	"notify.backup_recovered.title":        {"Backup wieder OK: {name}", "Backup OK again: {name}"},
	"notify.backup_recovered.message":      {"{integration} – letzter Lauf {last}", "{integration} – last run {last}"},
	"notify.integration_down.title":        {"Nicht erreichbar: {name}", "Unreachable: {name}"},
	"notify.integration_recovered.title":   {"Wieder erreichbar: {name}", "Reachable again: {name}"},
	"notify.integration_recovered.message": {"Die Abfrage funktioniert wieder.", "Polling works again."},
	"notify.threshold.title":               {"Schwellwert: {name} – {status}", "Threshold: {name} – {status}"},
	"notify.threshold.message":             {"{integration} – Wert {value}", "{integration} – value {value}"},
	"notify.test.title":                    {"labdeck: Testnachricht", "labdeck: test message"},
	"notify.test.message":                  {"Benachrichtigungen funktionieren.", "Notifications are working."},
	"status.ok":                            {"OK", "OK"},
	"status.warning":                       {"Warnung", "warning"},
	"status.error":                         {"Fehler", "error"},

	// Dashboard
	"dashboard.tile_invalid": {"unbekannte oder doppelte Kachel {id} bzw. Größe nicht 1–3", "unknown or duplicate tile {id} or size not 1–3"},

	// Proxmox VE
	"pve.unexpected":   {"Antwort ist keine PVE-API – Adresse mit Port 8006 prüfen", "The response is not the PVE API – check the address with port 8006"},
	"pve.no_nodes":     {"Das Token sieht keine Nodes – Rolle PVEAuditor auf / für das Token vergeben", "The token sees no nodes – give the token the role PVEAuditor on /"},
	"pve.node_missing": {"Node {nodes} nicht gefunden oder für das Token nicht sichtbar", "Node {nodes} not found or not visible to the token"},

	// Dockhand
	"dockhand.unexpected":  {"Antwort ist keine Dockhand-API – ist die Adresse die Dockhand-Oberfläche?", "The response is not the Dockhand API – is the address the Dockhand web interface?"},
	"dockhand.no_envs":     {"Dockhand kennt keine Umgebung (Docker-Host) oder das Token darf keine sehen", "Dockhand knows no environment (Docker host) or the token may not see any"},
	"dockhand.env_missing": {"Umgebung {names} in Dockhand nicht gefunden", "Environment {names} not found in Dockhand"},

	// USV (NUT)
	"nut.protocol":      {"Antwort ist kein NUT-Protokoll – läuft unter Host und Port upsd?", "The response is not the NUT protocol – is upsd running at host and port?"},
	"nut.access_denied": {"upsd verweigert den Zugriff ({code}) – Benutzer/Passwort aus upsd.users prüfen", "upsd denies access ({code}) – check user/password from upsd.users"},
	"nut.unknown_ups":   {"USV unbekannt – Namen aus „upsc -l“ verwenden", "Unknown UPS – use a name from \"upsc -l\""},
	"nut.no_ups":        {"upsd kennt keine USV", "upsd knows no UPS"},
	"nut.driver":        {"Der USV-Treiber liefert keine Daten ({code}) – Kabel und Treiber prüfen", "The UPS driver delivers no data ({code}) – check cable and driver"},
	"nut.error":         {"upsd meldet einen Fehler: {code}", "upsd reports an error: {code}"},

	"snmp.timeout":             {"Keine Antwort per SNMP – Host, Port, Community bzw. v3-Zugangsdaten und die Freigabe für labdeck auf dem Switch prüfen", "No SNMP response – check host, port, community or v3 credentials and whether the switch allows labdeck"},
	"snmp.v3_auth":             {"SNMPv3-Anmeldung abgelehnt – Benutzer, Protokolle und Passwörter prüfen", "SNMPv3 login rejected – check user, protocols and passwords"},
	"snmp.protocol":            {"Antwort ist kein gültiges SNMP", "The response is not valid SNMP"},
	"snmp.agent_error":         {"Der Switch meldet einen SNMP-Fehler: {code}", "The switch reports an SNMP error: {code}"},
	"pihole.login_failed":      {"Pi-hole lehnt das Passwort ab – ein App-Passwort aus Einstellungen → Web-Oberfläche / API verwenden", "Pi-hole rejects the password – use an app password from Settings → Web interface / API"},
	"pihole.totp":              {"Pi-hole verlangt Zwei-Faktor-Anmeldung – ein App-Passwort verwenden, das gilt ohne zweiten Faktor", "Pi-hole requires two-factor login – use an app password, it works without a second factor"},
	"pihole.password_required": {"Pi-hole verlangt ein Passwort – App-Passwort eintragen", "Pi-hole requires a password – enter an app password"},
	"redfish.no_system":        {"Der BMC meldet kein System unter /redfish/v1/Systems", "The BMC reports no system at /redfish/v1/Systems"},
	"redfish.bad_link":         {"Der BMC verweist auf eine ungültige Adresse", "The BMC links to an invalid address"},
	"snmp.passphrase_short":    {"{field}-Passwort muss mindestens 8 Zeichen haben", "{field} password needs at least 8 characters"},

	// Einstellungen
	"settings.resource_retention_range": {"Aufbewahrung der Rohdaten je Ressource muss zwischen {min} und {max} Tagen liegen", "Raw data retention per resource must be between {min} and {max} days"},
	"settings.notify_language_invalid":  {"Sprache der Benachrichtigungen muss de oder en sein", "The notification language must be de or en"},
	"settings.timezone_invalid":         {"unbekannte Zeitzone {name}", "unknown time zone {name}"},
	"settings.raw_retention_range":      {"Aufbewahrung der Rohdaten muss zwischen {min} und {max} Tagen liegen", "Raw data retention must be between {min} and {max} days"},
	"settings.daily_retention_range":    {"Aufbewahrung der Tageswerte muss zwischen {min} und {max} Tagen liegen", "Daily value retention must be between {min} and {max} days"},
	"settings.backup_max_age_range":     {"Höchstalter von Backups muss zwischen {min} und {max} Stunden liegen", "The maximum backup age must be between {min} and {max} hours"},
}
