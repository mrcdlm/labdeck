package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mrcdlm/labdeck/internal/events"
	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/secrets"
	"github.com/mrcdlm/labdeck/internal/store"
)

// Grenzen je Instanz.
const (
	DefaultInterval = 60 * time.Second
	MinInterval     = 15 * time.Second
	MaxInterval     = 24 * time.Hour
	DefaultTimeout  = 10 * time.Second
	MinTimeout      = time.Second
	MaxTimeout      = 120 * time.Second
	maxNameLen      = 100
)

type Store interface {
	CreateIntegration(ctx context.Context, it store.Integration, seal func(id int64) (string, error)) (store.Integration, error)
	UpdateIntegration(ctx context.Context, it store.Integration) (store.Integration, error)
	DeleteIntegration(ctx context.Context, id int64) error
	GetIntegration(ctx context.Context, id int64) (store.Integration, error)
	ListIntegrations(ctx context.Context) ([]store.Integration, error)
	SavePollResult(ctx context.Context, id int64, r store.PollResult) error
}

// ErrNotFound: Integration existiert nicht.
var ErrNotFound = i18n.E("integration.not_found_id")

// ValidationError sind Eingabefehler, die dem Benutzer angezeigt werden.
type ValidationError struct{ Msgs []i18n.Msg }

func (e *ValidationError) Error() string { return i18n.Join(i18n.EN, e.Msgs) }

// Input ist eine Integration, wie sie das Formular schickt.
type Input struct {
	Type            string         `json:"type"`
	Name            string         `json:"name"`
	Enabled         bool           `json:"enabled"`
	IntervalSeconds int            `json:"interval_seconds"`
	TimeoutSeconds  int            `json:"timeout_seconds"`
	TLSVerify       bool           `json:"tls_verify"`
	Config          map[string]any `json:"config"`
	// Secrets: neue Werte; fehlend oder leer = unverändert
	Secrets map[string]string `json:"secrets"`
	// ClearSecrets: diese Secret-Felder leeren
	ClearSecrets []string `json:"clear_secrets"`
}

// SecretState: Zugangsdaten werden nie ausgeliefert, nur ob sie gesetzt sind
// und – bei Verweisen – der Name der Env-Variablen.
type SecretState struct {
	Set bool   `json:"set"`
	Ref string `json:"ref,omitempty"`
}

// View ist eine Integration für die API (ohne Geheimnisse).
type View struct {
	ID              int64                  `json:"id"`
	Type            string                 `json:"type"`
	Name            string                 `json:"name"`
	Enabled         bool                   `json:"enabled"`
	IntervalSeconds int                    `json:"interval_seconds"`
	TimeoutSeconds  int                    `json:"timeout_seconds"`
	TLSVerify       bool                   `json:"tls_verify"`
	Config          map[string]any         `json:"config"`
	Secrets         map[string]SecretState `json:"secrets"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`

	Status        Status     `json:"status"`
	LastPollAt    *time.Time `json:"last_poll_at,omitempty"`
	LastSuccessAt *time.Time `json:"last_success_at,omitempty"`
	LastError     i18n.Msg   `json:"-"`
	LastDuration  int64      `json:"last_duration_ms"`
	Snapshot      *Snapshot  `json:"snapshot,omitempty"`
}

// TestResult ist das Ergebnis von „Verbindung testen“.
type TestResult struct {
	OK       bool     `json:"ok"`
	Error    i18n.Msg `json:"-"`
	Duration int64    `json:"duration_ms"`
	Checks   int      `json:"checks"`
	Backups  int      `json:"backups"`
	Metrics  int      `json:"metrics"`
}

type Manager struct {
	reg   *Registry
	store Store
	box   *secrets.Box // nil = kein LABDECK_SECRET_KEY
	env   secrets.Env
	log   *slog.Logger
	// Pub erhält TopicIntegrations nach jeder Änderung und Abfrage (nil = aus).
	Pub events.Publisher
	// OnChange wird nach Anlegen/Ändern/Löschen aufgerufen (Scheduler abgleichen).
	OnChange func(id int64)
	now      func() time.Time

	mu      sync.Mutex
	polling map[int64]*sync.Mutex // eine Abfrage je Integration gleichzeitig
	clients map[int64]clientEntry // HTTP-Client je Integration (Verbindungen wiederverwenden)
}

type clientEntry struct {
	timeout time.Duration
	verify  bool
	client  *http.Client
}

func NewManager(reg *Registry, st Store, box *secrets.Box, env secrets.Env, log *slog.Logger) *Manager {
	return &Manager{reg: reg, store: st, box: box, env: env, log: log, now: time.Now,
		polling: map[int64]*sync.Mutex{}, clients: map[int64]clientEntry{}}
}

// Registry liefert die verfügbaren Typen.
func (m *Manager) Registry() *Registry { return m.reg }

// HasKey meldet, ob Zugangsdaten verschlüsselt gespeichert werden können.
func (m *Manager) HasKey() bool { return m.box != nil }

func sealAD(id int64) string { return "labdeck/integration/" + strconv.FormatInt(id, 10) }

// stored ist der Inhalt der Spalte config: nicht geheime Werte und
// Secret-Verweise (${SECRET_…}) unter dem Feldnamen.
type stored map[string]any

// split trennt gespeicherte Werte in Formularwerte und Secret-Verweise.
func split(schema ConfigSchema, raw string) (values map[string]any, refs map[string]string) {
	values, refs = map[string]any{}, map[string]string{}
	var all stored
	_ = json.Unmarshal([]byte(raw), &all)
	for k, v := range all {
		f, ok := schema.Field(k)
		if !ok {
			continue
		}
		if f.Type == FieldSecret {
			if s, ok := v.(string); ok {
				refs[k] = s
			}
			continue
		}
		values[k] = v
	}
	return values, refs
}

// openSecrets entschlüsselt die gespeicherten Secrets (leer = keine).
func (m *Manager) openSecrets(it store.Integration) (map[string]string, error) {
	out := map[string]string{}
	if it.Secrets == "" {
		return out, nil
	}
	plain, err := m.box.Open(it.Secrets, sealAD(it.ID))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(plain, &out); err != nil {
		return nil, secrets.ErrDecrypt
	}
	return out, nil
}

// prepared ist eine geprüfte Eingabe, bereit zum Speichern bzw. Testen.
type prepared struct {
	integ    Integration
	rec      store.Integration // ohne ID/Secrets
	values   map[string]any
	refs     map[string]string
	literals map[string]string // Klartext-Secrets (werden verschlüsselt)
}

// prepare prüft in gegen das Schema. existing (nil = neu) liefert
// unveränderte Secrets.
func (m *Manager) prepare(in Input, existing *store.Integration, lang i18n.Lang) (prepared, error) {
	var msgs []i18n.Msg
	add := func(code string, kv ...string) { msgs = append(msgs, i18n.M(code, kv...)) }

	if existing != nil && in.Type != "" && in.Type != existing.Type {
		add("integration.type_change")
	}
	typ := in.Type
	if existing != nil {
		typ = existing.Type
	}
	integ, ok := m.reg.Get(typ)
	if !ok {
		return prepared{}, &ValidationError{Msgs: []i18n.Msg{i18n.M("integration.type_unknown", "type", typ)}}
	}
	schema := integ.Schema()

	name := strings.TrimSpace(in.Name)
	if name == "" || len([]rune(name)) > maxNameLen || strings.ContainsAny(name, "\r\n") {
		add("integration.name_invalid")
	}
	interval := time.Duration(in.IntervalSeconds) * time.Second
	if in.IntervalSeconds == 0 {
		interval = DefaultInterval
	}
	if interval < MinInterval || interval > MaxInterval {
		add("integration.interval_range", "min", strconv.Itoa(int(MinInterval.Seconds())), "max", strconv.Itoa(int(MaxInterval.Seconds())))
	}
	timeout := time.Duration(in.TimeoutSeconds) * time.Second
	if in.TimeoutSeconds == 0 {
		timeout = DefaultTimeout
	}
	intervalOK := interval >= MinInterval && interval <= MaxInterval
	if timeout < MinTimeout || timeout > MaxTimeout || (intervalOK && timeout >= interval) {
		add("integration.timeout_range", "min", strconv.Itoa(int(MinTimeout.Seconds())), "max", strconv.Itoa(int(MaxTimeout.Seconds())))
	}

	values, vmsgs := schema.Normalize(in.Config, lang)
	msgs = append(msgs, vmsgs...)

	// Secrets: bisherige Werte als Ausgangspunkt
	refs, literals := map[string]string{}, map[string]string{}
	if existing != nil {
		_, refs = split(schema, existing.Config)
		old, err := m.openSecrets(*existing)
		switch {
		case err == nil:
			literals = old
		case errors.Is(err, secrets.ErrNoKey), errors.Is(err, secrets.ErrDecrypt):
			// unbrauchbar – nur Pflichtfelder ohne neuen Wert melden (unten)
		default:
			return prepared{}, err
		}
	}
	for k := range in.Secrets {
		if f, ok := schema.Field(k); !ok || f.Type != FieldSecret {
			add("integration.field_unknown", "field", k)
		}
	}
	for _, k := range in.ClearSecrets {
		delete(refs, k)
		delete(literals, k)
	}
	for _, f := range schema.Fields {
		if f.Type != FieldSecret {
			continue
		}
		v := strings.TrimSpace(in.Secrets[f.Key])
		switch {
		case v == "":
		case secrets.IsRef(v):
			if err := secrets.CheckRefs(v); err != nil {
				add("integration.field_secret_ref", "field", f.Label.In(lang), "detail", i18n.Nest(i18n.FromError(err)))
				continue
			}
			refs[f.Key] = v
			delete(literals, f.Key)
		default:
			literals[f.Key] = v
			delete(refs, f.Key)
		}
		if f.Required && refs[f.Key] == "" && literals[f.Key] == "" {
			if existing != nil && existing.Secrets != "" && m.decryptFails(*existing) {
				add("integration.field_secret_ref", "field", f.Label.In(lang), "detail", i18n.Nest(i18n.FromError(secrets.ErrDecrypt)))
			} else {
				add("integration.field_required", "field", f.Label.In(lang))
			}
		}
	}
	if len(literals) > 0 && m.box == nil {
		msgs = append(msgs, i18n.FromError(secrets.ErrNoKey))
	}
	if len(msgs) > 0 {
		return prepared{}, &ValidationError{Msgs: msgs}
	}

	all := stored{}
	for k, v := range values {
		all[k] = v
	}
	for k, v := range refs {
		all[k] = v
	}
	cfgJSON, err := json.Marshal(all)
	if err != nil {
		return prepared{}, err
	}
	return prepared{
		integ: integ,
		rec: store.Integration{Type: typ, Name: name, Enabled: in.Enabled, Interval: interval, Timeout: timeout,
			TLSVerify: in.TLSVerify, Config: string(cfgJSON)},
		values: values, refs: refs, literals: literals,
	}, nil
}

func (m *Manager) decryptFails(it store.Integration) bool {
	_, err := m.openSecrets(it)
	return err != nil
}

func (m *Manager) seal(literals map[string]string) func(id int64) (string, error) {
	return func(id int64) (string, error) {
		if len(literals) == 0 {
			return "", nil
		}
		plain, err := json.Marshal(literals)
		if err != nil {
			return "", err
		}
		return m.box.Seal(plain, sealAD(id))
	}
}

func (m *Manager) Create(ctx context.Context, in Input, lang i18n.Lang) (View, error) {
	p, err := m.prepare(in, nil, lang)
	if err != nil {
		return View{}, err
	}
	it, err := m.store.CreateIntegration(ctx, p.rec, m.seal(p.literals))
	if err != nil {
		return View{}, err
	}
	m.log.Info("Integration angelegt", "id", it.ID, "type", it.Type, "name", it.Name)
	m.changed(it.ID)
	return m.view(it), nil
}

func (m *Manager) Update(ctx context.Context, id int64, in Input, lang i18n.Lang) (View, error) {
	existing, err := m.get(ctx, id)
	if err != nil {
		return View{}, err
	}
	p, err := m.prepare(in, &existing, lang)
	if err != nil {
		return View{}, err
	}
	rec := p.rec
	rec.ID = id
	if rec.Secrets, err = m.seal(p.literals)(id); err != nil {
		return View{}, err
	}
	it, err := m.store.UpdateIntegration(ctx, rec)
	if errors.Is(err, store.ErrNotFound) {
		return View{}, ErrNotFound
	}
	if err != nil {
		return View{}, err
	}
	m.dropClient(id)
	m.log.Info("Integration geändert", "id", id, "type", it.Type, "name", it.Name, "enabled", it.Enabled)
	m.changed(id)
	return m.view(it), nil
}

func (m *Manager) Delete(ctx context.Context, id int64) error {
	err := m.store.DeleteIntegration(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	m.dropClient(id)
	m.log.Info("Integration gelöscht", "id", id)
	m.changed(id)
	return nil
}

func (m *Manager) changed(id int64) {
	events.Publish(m.Pub, events.TopicIntegrations)
	if m.OnChange != nil {
		m.OnChange(id)
	}
}

func (m *Manager) get(ctx context.Context, id int64) (store.Integration, error) {
	it, err := m.store.GetIntegration(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return it, ErrNotFound
	}
	return it, err
}

func (m *Manager) Get(ctx context.Context, id int64) (View, error) {
	it, err := m.get(ctx, id)
	if err != nil {
		return View{}, err
	}
	return m.view(it), nil
}

func (m *Manager) List(ctx context.Context) ([]View, error) {
	list, err := m.store.ListIntegrations(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]View, 0, len(list))
	for _, it := range list {
		out = append(out, m.view(it))
	}
	return out, nil
}

// Schedules liefert die aktiven Integrationen mit Intervall (für den Scheduler).
func (m *Manager) Schedules(ctx context.Context) (map[int64]time.Duration, error) {
	list, err := m.store.ListIntegrations(ctx)
	if err != nil {
		return nil, err
	}
	out := map[int64]time.Duration{}
	for _, it := range list {
		if it.Enabled {
			out[it.ID] = it.Interval
		}
	}
	return out, nil
}

func (m *Manager) view(it store.Integration) View {
	v := View{ID: it.ID, Type: it.Type, Name: it.Name, Enabled: it.Enabled,
		IntervalSeconds: int(it.Interval.Seconds()), TimeoutSeconds: int(it.Timeout.Seconds()), TLSVerify: it.TLSVerify,
		Config: map[string]any{}, Secrets: map[string]SecretState{}, CreatedAt: it.CreatedAt, UpdatedAt: it.UpdatedAt,
		Status: Status(it.Status), LastPollAt: it.LastPollAt, LastSuccessAt: it.LastSuccessAt,
		LastError: i18n.Decode(it.LastError), LastDuration: it.LastDuration.Milliseconds()}
	if !it.Enabled {
		v.Status = StatusPaused
	}
	if it.Snapshot != "" {
		var s Snapshot
		if json.Unmarshal([]byte(it.Snapshot), &s) == nil {
			v.Snapshot = &s
		}
	}
	integ, ok := m.reg.Get(it.Type)
	if !ok {
		return v
	}
	schema := integ.Schema()
	values, refs := split(schema, it.Config)
	v.Config = values
	// Ob ein verschlüsselter Wert existiert, lässt sich ohne Entschlüsseln
	// nicht je Feld sagen – daher öffnen (der Klartext verlässt diese Funktion nicht).
	literals, _ := m.openSecrets(it)
	for _, f := range schema.Fields {
		if f.Type != FieldSecret {
			continue
		}
		st := SecretState{Ref: refs[f.Key]}
		st.Set = st.Ref != "" || literals[f.Key] != ""
		if !st.Set && it.Secrets != "" && literals == nil {
			st.Set = true // vorhanden, aber nicht entschlüsselbar – Fehler zeigt die Abfrage
		}
		v.Secrets[f.Key] = st
	}
	return v
}

// client liefert den HTTP-Client der Integration (wiederverwendet, solange
// Timeout und TLS-Einstellung gleich bleiben).
func (m *Manager) client(id int64, timeout time.Duration, verify bool) *http.Client {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.clients[id]; ok && e.timeout == timeout && e.verify == verify {
		return e.client
	}
	if e, ok := m.clients[id]; ok {
		e.client.CloseIdleConnections()
	}
	c := NewHTTPClient(timeout, verify)
	if id != 0 {
		m.clients[id] = clientEntry{timeout: timeout, verify: verify, client: c}
	}
	return c
}

func (m *Manager) dropClient(id int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.clients[id]; ok {
		e.client.CloseIdleConnections()
		delete(m.clients, id)
	}
}

// config baut die Konfiguration für Test/Collect: Verweise auflösen,
// Klartext-Secrets übernehmen.
func (m *Manager) config(id int64, values map[string]any, refs, literals map[string]string, timeout time.Duration, verify bool) (Config, error) {
	sec := map[string]string{}
	for k, v := range literals {
		sec[k] = v
	}
	for k, ref := range refs {
		v, err := secrets.Expand(ref, m.env)
		if err != nil {
			return Config{}, err
		}
		sec[k] = v
	}
	return Config{Values: values, Secrets: sec, HTTP: m.client(id, timeout, verify), Timeout: timeout}, nil
}

// Test prüft eine (noch nicht gespeicherte) Eingabe. id != 0: Secrets, die
// das Formular leer lässt, kommen aus der gespeicherten Integration.
func (m *Manager) Test(ctx context.Context, id int64, in Input, lang i18n.Lang) (TestResult, error) {
	var existing *store.Integration
	if id != 0 {
		it, err := m.get(ctx, id)
		if err != nil {
			return TestResult{}, err
		}
		existing = &it
	}
	p, err := m.prepare(in, existing, lang)
	if err != nil {
		return TestResult{}, err
	}
	cfg, err := m.config(0, p.values, p.refs, p.literals, p.rec.Timeout, p.rec.TLSVerify)
	if err != nil {
		return TestResult{Error: i18n.FromError(err)}, nil
	}
	defer cfg.HTTP.CloseIdleConnections()

	start := m.now()
	ctx, cancel := context.WithTimeout(ctx, 2*p.rec.Timeout+time.Second)
	defer cancel()
	if err := p.integ.Test(ctx, cfg); err != nil {
		return TestResult{Error: i18n.FromError(err), Duration: time.Since(start).Milliseconds()}, nil
	}
	res := TestResult{OK: true}
	// Kurzer Überblick, was die Quelle liefert („12 Checks, 3 Backups“).
	if snap, err := p.integ.Collect(ctx, cfg); err == nil {
		res.Checks, res.Backups, res.Metrics = len(snap.Checks), len(snap.Backups), len(snap.Metrics)
	}
	res.Duration = time.Since(start).Milliseconds()
	return res, nil
}

func (m *Manager) pollLock(id int64) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.polling[id]
	if !ok {
		l = &sync.Mutex{}
		m.polling[id] = l
	}
	return l
}

// Poll fragt eine Integration ab und speichert Ergebnis, Statuswechsel und
// Kennzahlen. Ein Fehler der Quelle ist kein Fehler von Poll (er wird als
// Status gespeichert); Poll meldet nur Fehler der eigenen Speicherung.
func (m *Manager) Poll(ctx context.Context, id int64) error {
	l := m.pollLock(id)
	l.Lock()
	defer l.Unlock()

	it, err := m.get(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return nil // inzwischen gelöscht
	}
	if err != nil {
		return err
	}

	start := m.now()
	snap, collectErr := m.collect(ctx, it)
	at := m.now()
	res := store.PollResult{At: at, Duration: at.Sub(start), Status: string(StatusOK)}

	prevStatus := Status(it.Status)
	if collectErr != nil {
		if errors.Is(collectErr, context.Canceled) && ctx.Err() != nil {
			return nil // Herunterfahren
		}
		msg := i18n.FromError(collectErr)
		res.Status, res.Error = string(StatusError), i18n.Encode(msg)
		if prevStatus != StatusError {
			m.log.Warn("Integration nicht erreichbar", "id", id, "name", it.Name, "err", i18n.T(i18n.EN, msg))
		}
	} else {
		b, err := json.Marshal(snap)
		if err != nil {
			return err
		}
		res.Snapshot = string(b)
		var prev Snapshot
		if it.Snapshot != "" {
			_ = json.Unmarshal([]byte(it.Snapshot), &prev)
		}
		res.Changes = itemChanges(prev, snap, at)
		for _, mt := range snap.Metrics {
			res.Samples = append(res.Samples, store.MetricSample{Key: mt.Key, Value: mt.Value})
		}
		if prevStatus == StatusError {
			m.log.Info("Integration wieder erreichbar", "id", id, "name", it.Name)
		}
	}
	// Nur echte Wechsel; die jeweils letzte Fehlermeldung steht an der Integration.
	if Status(res.Status) != prevStatus {
		res.Changes = append([]store.StatusChange{{Kind: "integration", ItemName: it.Name,
			From: string(prevStatus), To: res.Status, Message: res.Error, At: at}}, res.Changes...)
	}

	if err := m.store.SavePollResult(context.WithoutCancel(ctx), id, res); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("save poll result: %w", err)
	}
	m.log.Debug("Integration abgefragt", "id", id, "status", res.Status, "duration", res.Duration)
	events.Publish(m.Pub, events.TopicIntegrations)
	return nil
}

func (m *Manager) collect(ctx context.Context, it store.Integration) (Snapshot, error) {
	integ, ok := m.reg.Get(it.Type)
	if !ok {
		return Snapshot{}, i18n.E("integration.type_unknown", "type", it.Type)
	}
	values, refs := split(integ.Schema(), it.Config)
	literals, err := m.openSecrets(it)
	if err != nil {
		return Snapshot{}, err
	}
	cfg, err := m.config(it.ID, values, refs, literals, it.Timeout, it.TLSVerify)
	if err != nil {
		return Snapshot{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*it.Timeout+time.Second)
	defer cancel()
	return integ.Collect(ctx, cfg)
}

// itemChanges vergleicht zwei Snapshots: neue Einträge und geänderte Status
// von Checks, Backups und Kennzahlen.
func itemChanges(prev, cur Snapshot, at time.Time) []store.StatusChange {
	var out []store.StatusChange
	add := func(kind, id, name string, from, to Status) {
		if from == to {
			return
		}
		if from == "" {
			from = StatusUnknown
			if to == StatusUnknown {
				return
			}
		}
		out = append(out, store.StatusChange{Kind: kind, ItemID: id, ItemName: name, From: string(from), To: string(to), At: at})
	}
	checks := map[string]Status{}
	for _, c := range prev.Checks {
		checks[c.ID] = c.Status
	}
	for _, c := range cur.Checks {
		add("check", c.ID, c.Name, checks[c.ID], c.Status)
	}
	backups := map[string]Status{}
	for _, b := range prev.Backups {
		backups[b.ID] = b.Result
	}
	for _, b := range cur.Backups {
		add("backup", b.ID, b.Name, backups[b.ID], b.Result)
	}
	metrics := map[string]Status{}
	for _, mt := range prev.Metrics {
		metrics[mt.Key] = mt.Status
	}
	for _, mt := range cur.Metrics {
		add("metric", mt.Key, mt.Name, metrics[mt.Key], mt.Status)
	}
	slices.SortStableFunc(out, func(a, b store.StatusChange) int { return strings.Compare(a.Kind, b.Kind) })
	return out
}
