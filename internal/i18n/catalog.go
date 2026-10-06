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

	// Einstellungen
	"settings.timezone_invalid":      {"unbekannte Zeitzone {name}", "unknown time zone {name}"},
	"settings.raw_retention_range":   {"Aufbewahrung der Rohdaten muss zwischen {min} und {max} Tagen liegen", "Raw data retention must be between {min} and {max} days"},
	"settings.daily_retention_range": {"Aufbewahrung der Tageswerte muss zwischen {min} und {max} Tagen liegen", "Daily value retention must be between {min} and {max} days"},
}
