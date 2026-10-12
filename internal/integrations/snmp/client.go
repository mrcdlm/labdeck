package snmp

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/mrcdlm/labdeck/internal/i18n"
	"github.com/mrcdlm/labdeck/internal/integrations"
)

// OIDs aus SNMPv2-MIB (RFC 3418), IF-MIB (RFC 2863) und ENTITY-MIB (RFC 6933).
// Nur Standard-MIBs: so funktioniert jeder Switch, nicht nur ein Modell.
const (
	oidSysDescr  = ".1.3.6.1.2.1.1.1.0"
	oidSysUpTime = ".1.3.6.1.2.1.1.3.0"
	oidSysName   = ".1.3.6.1.2.1.1.5.0"

	oidIfEntry  = ".1.3.6.1.2.1.2.2.1"      // ifTable
	oidIfXEntry = ".1.3.6.1.2.1.31.1.1.1"   // ifXTable
	oidEntPhys  = ".1.3.6.1.2.1.47.1.1.1.1" // entPhysicalTable
	oidEntAlias = ".1.3.6.1.2.1.47.1.3.2.1.2"
)

// Spalten von ifTable, ifXTable und entPhysicalTable.
const (
	colIfDescr       = oidIfEntry + ".2"
	colIfType        = oidIfEntry + ".3"
	colIfAdminStatus = oidIfEntry + ".7"
	colIfOperStatus  = oidIfEntry + ".8"
	colIfInDiscards  = oidIfEntry + ".13"
	colIfInErrors    = oidIfEntry + ".14"
	colIfOutDiscards = oidIfEntry + ".19"
	colIfOutErrors   = oidIfEntry + ".20"

	colIfName        = oidIfXEntry + ".1"
	colIfHCInOctets  = oidIfXEntry + ".6"
	colIfHCOutOctets = oidIfXEntry + ".10"
	colIfHighSpeed   = oidIfXEntry + ".15"
	colIfAlias       = oidIfXEntry + ".18"

	colEntDescr       = oidEntPhys + ".2"
	colEntContainedIn = oidEntPhys + ".4"
	colEntClass       = oidEntPhys + ".5"
	colEntRelPos      = oidEntPhys + ".6"
	colEntName        = oidEntPhys + ".7"
	colEntSoftware    = oidEntPhys + ".10"
	colEntSerial      = oidEntPhys + ".11"
	colEntModel       = oidEntPhys + ".13"
)

var ifColumns = []string{colIfDescr, colIfType, colIfAdminStatus, colIfOperStatus, colIfInDiscards, colIfInErrors,
	colIfOutDiscards, colIfOutErrors, colIfName, colIfHCInOctets, colIfHCOutOctets, colIfHighSpeed, colIfAlias}

var entColumns = []string{colEntDescr, colEntContainedIn, colEntClass, colEntRelPos, colEntName, colEntSoftware,
	colEntSerial, colEntModel}

// table: Spalte → Index (Rest der OID, z. B. "17") → Wert.
type table map[string]map[string]any

func (t table) str(col, idx string) string {
	switch v := t[col][idx].(type) {
	case string:
		return v
	case []byte:
		return string(v)
	}
	return ""
}

func (t table) num(col, idx string) (uint64, bool) {
	v, ok := t[col][idx]
	if !ok {
		return 0, false
	}
	b := gosnmp.ToBigInt(v)
	if b == nil || b.Sign() < 0 {
		return 0, false
	}
	return b.Uint64(), true
}

// device sind die Rohdaten einer Abfrage.
type device struct {
	descr, name string
	uptime      time.Duration
	ifs         table
	ent         table             // nil = keine ENTITY-MIB
	alias       map[string]string // Entity-Index → ifIndex
	at          time.Time
}

// connect baut die Verbindung aus der Konfiguration auf. SNMP läuft über
// UDP: ein falscher Community-String bleibt unbeantwortet (Timeout).
func connect(ctx context.Context, cfg integrations.Config) (*gosnmp.GoSNMP, error) {
	host := strings.TrimSpace(cfg.String("host"))
	if host == "" {
		return nil, i18n.E("integration.field_required", "field", "Host")
	}
	port := defaultPort
	if cfg.Has("port") {
		port = cfg.Int("port")
	}
	timeout := cfg.Timeout / 3
	if timeout < 500*time.Millisecond {
		timeout = 500 * time.Millisecond
	}
	g := &gosnmp.GoSNMP{
		Target:         host,
		Port:           uint16(port), //nolint:gosec // Schema begrenzt auf 1–65535
		Transport:      "udp",
		Timeout:        timeout,
		Retries:        1,
		Context:        ctx,
		MaxOids:        gosnmp.MaxOids,
		MaxRepetitions: 40,
		Logger:         gosnmp.NewLogger(log.New(io.Discard, "", 0)),
	}
	switch cfg.String("version") {
	case "v3":
		sp, flags, err := usm(cfg)
		if err != nil {
			return nil, err
		}
		g.Version = gosnmp.Version3
		g.SecurityModel = gosnmp.UserSecurityModel
		g.MsgFlags = flags
		g.SecurityParameters = sp
	default:
		g.Version = gosnmp.Version2c
		g.Community = cfg.Secret("community")
		if g.Community == "" {
			return nil, i18n.E("integration.field_required", "field", "Community")
		}
	}
	if err := g.Connect(); err != nil {
		return nil, integrations.RequestError(err)
	}
	return g, nil
}

var authProtocols = map[string]gosnmp.SnmpV3AuthProtocol{
	"md5": gosnmp.MD5, "sha": gosnmp.SHA, "sha224": gosnmp.SHA224, "sha256": gosnmp.SHA256,
	"sha384": gosnmp.SHA384, "sha512": gosnmp.SHA512,
}

var privProtocols = map[string]gosnmp.SnmpV3PrivProtocol{
	"des": gosnmp.DES, "aes": gosnmp.AES, "aes192": gosnmp.AES192, "aes256": gosnmp.AES256,
	"aes192c": gosnmp.AES192C, "aes256c": gosnmp.AES256C,
}

// usm liefert die v3-Sicherheitsparameter (User-based Security Model).
func usm(cfg integrations.Config) (*gosnmp.UsmSecurityParameters, gosnmp.SnmpV3MsgFlags, error) {
	user := strings.TrimSpace(cfg.String("user"))
	if user == "" {
		return nil, 0, i18n.E("integration.field_required", "field", "User")
	}
	sp := &gosnmp.UsmSecurityParameters{UserName: user}
	level := cfg.String("level")
	if level == "noAuthNoPriv" {
		return sp, gosnmp.NoAuthNoPriv, nil
	}
	sp.AuthenticationProtocol = authProtocols[cfg.String("auth_protocol")]
	if sp.AuthenticationProtocol == 0 {
		sp.AuthenticationProtocol = gosnmp.SHA
	}
	sp.AuthenticationPassphrase = cfg.Secret("auth_password")
	if len(sp.AuthenticationPassphrase) < 8 {
		return nil, 0, i18n.E("snmp.passphrase_short", "field", "Auth")
	}
	if level == "authNoPriv" {
		return sp, gosnmp.AuthNoPriv, nil
	}
	sp.PrivacyProtocol = privProtocols[cfg.String("priv_protocol")]
	if sp.PrivacyProtocol == 0 {
		sp.PrivacyProtocol = gosnmp.AES
	}
	sp.PrivacyPassphrase = cfg.Secret("priv_password")
	if len(sp.PrivacyPassphrase) < 8 {
		return nil, 0, i18n.E("snmp.passphrase_short", "field", "Priv")
	}
	return sp, gosnmp.AuthPriv, nil
}

// snmpError übersetzt Fehler von gosnmp, ohne Zugangsdaten zu nennen.
func snmpError(ctx context.Context, err error) error {
	var ne net.Error
	msg := strings.ToLower(err.Error())
	switch {
	case ctx.Err() != nil, errors.As(err, &ne) && ne.Timeout(), strings.Contains(msg, "timeout"):
		return i18n.E("snmp.timeout")
	case strings.Contains(msg, "unknown user"), strings.Contains(msg, "wrong digest"),
		strings.Contains(msg, "authentication"), strings.Contains(msg, "decryption"):
		return i18n.E("snmp.v3_auth")
	case strings.Contains(msg, "refused"):
		return i18n.E("integration.refused")
	}
	return i18n.E("snmp.protocol")
}

// fetchSystem liest sysDescr, sysName und sysUpTime (auch für „Verbindung testen“).
func fetchSystem(ctx context.Context, g *gosnmp.GoSNMP, d *device) error {
	res, err := g.Get([]string{oidSysDescr, oidSysName, oidSysUpTime})
	if err != nil {
		return snmpError(ctx, err)
	}
	if res.Error != gosnmp.NoError {
		return i18n.E("snmp.agent_error", "code", res.Error.String())
	}
	for _, v := range res.Variables {
		switch v.Name {
		case oidSysDescr:
			d.descr = strings.TrimSpace(asString(v.Value))
		case oidSysName:
			d.name = strings.TrimSpace(asString(v.Value))
		case oidSysUpTime:
			if b := gosnmp.ToBigInt(v.Value); b != nil {
				d.uptime = time.Duration(b.Int64()) * 10 * time.Millisecond // TimeTicks = 1/100 s
			}
		}
	}
	return nil
}

func asString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	}
	return ""
}

// walk liest Spalten per GETBULK in eine Tabelle.
func walk(ctx context.Context, g *gosnmp.GoSNMP, cols []string) (table, error) {
	t := table{}
	for _, col := range cols {
		m := map[string]any{}
		err := g.BulkWalk(col, func(pdu gosnmp.SnmpPDU) error {
			idx, ok := strings.CutPrefix(pdu.Name, col+".")
			if !ok {
				return nil
			}
			switch pdu.Type {
			case gosnmp.NoSuchObject, gosnmp.NoSuchInstance, gosnmp.EndOfMibView, gosnmp.Null:
				return nil
			}
			m[idx] = pdu.Value
			return nil
		})
		if err != nil {
			return nil, snmpError(ctx, err)
		}
		t[col] = m
	}
	return t, nil
}

// entAliases liest entAliasMappingIdentifier: Entity-Index → ifIndex.
func entAliases(ctx context.Context, g *gosnmp.GoSNMP) (map[string]string, error) {
	out := map[string]string{}
	err := g.BulkWalk(oidEntAlias, func(pdu gosnmp.SnmpPDU) error {
		idx, ok := strings.CutPrefix(pdu.Name, oidEntAlias+".")
		if !ok || pdu.Type != gosnmp.ObjectIdentifier {
			return nil
		}
		ent, _, _ := strings.Cut(idx, ".") // <entPhysicalIndex>.<entLogicalIndex>
		target, _ := pdu.Value.(string)
		if ifIndex, ok := strings.CutPrefix(target, ".1.3.6.1.2.1.2.2.1.1."); ok {
			if _, err := strconv.Atoi(ifIndex); err == nil {
				out[ent] = ifIndex
			}
		}
		return nil
	})
	if err != nil {
		return nil, snmpError(ctx, err)
	}
	return out, nil
}
