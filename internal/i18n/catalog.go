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

	// Einstellungen
	"settings.timezone_invalid":      {"unbekannte Zeitzone {name}", "unknown time zone {name}"},
	"settings.raw_retention_range":   {"Aufbewahrung der Rohdaten muss zwischen {min} und {max} Tagen liegen", "Raw data retention must be between {min} and {max} days"},
	"settings.daily_retention_range": {"Aufbewahrung der Tageswerte muss zwischen {min} und {max} Tagen liegen", "Daily value retention must be between {min} and {max} days"},
	"settings.backup_max_age_range":  {"Höchstalter von Backups muss zwischen {min} und {max} Stunden liegen", "The maximum backup age must be between {min} and {max} hours"},
}
