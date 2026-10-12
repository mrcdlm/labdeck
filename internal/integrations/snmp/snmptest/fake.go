// Package snmptest ist ein nachgebauter SNMP-Agent (v2c, UDP) für Tests und
// den Mock-Server: Get, GetNext und GetBulk auf einer OID-Tabelle mit
// steuerbaren Ports und Zählern. Falsche Community = keine Antwort, wie bei
// echten Geräten.
package snmptest

import (
	"cmp"
	"fmt"
	"io"
	"log"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/gosnmp/gosnmp"
)

const (
	sys     = ".1.3.6.1.2.1.1"
	ifEntry = ".1.3.6.1.2.1.2.2.1"
	ifX     = ".1.3.6.1.2.1.31.1.1.1"
	entPhys = ".1.3.6.1.2.1.47.1.1.1.1"
	entMap  = ".1.3.6.1.2.1.47.1.3.2.1.2"
)

// Port eines Profils.
type Port struct {
	Name, Alias string
	Type        int  // ifType, 6 = Ethernet, 161 = LAG
	SFP         bool // in der ENTITY-MIB als SFP beschrieben
	Speed       uint32
	Up, Admin   bool
}

// Profile beschreibt einen Switch.
type Profile struct {
	Descr, Name, Model, Firmware, Serial string
	Entity                               bool // ENTITY-MIB vorhanden
	Ports                                []Port
	Extra                                []Port // weitere Interfaces (VLAN, CPU …), werden nicht als Port gezählt
}

// C1300 ist ein Cisco Catalyst 1300-24P-4G mit ENTITY-MIB: 24 RJ45, 4 SFP,
// ein Port-Channel; Ports mit Beschreibung sind teils down.
func C1300() Profile {
	p := Profile{
		Descr:    "C1300-24P-4G 24-Port Gigabit PoE+ Managed Switch, 4 Gigabit SFP",
		Name:     "core-switch",
		Model:    "C1300-24P-4G",
		Firmware: "4.1.6.53",
		Serial:   "FOC2734Y0AB",
		Entity:   true,
	}
	aliases := map[int]string{1: "pve-01", 2: "pve-02", 3: "nas", 4: "pbs", 8: "ap-living", 12: "printer", 24: "uplink-router"}
	for n := 1; n <= 28; n++ {
		port := Port{Name: fmt.Sprintf("gi1/0/%d", n), Type: 6, Speed: 1000, Up: true, Admin: true, Alias: aliases[n], SFP: n > 24}
		switch {
		case n == 12: // Drucker aus: überwachter Port down
			port.Up = false
		case n >= 14 && n <= 22 || n >= 26:
			port.Up = false
		case n == 23:
			port.Admin, port.Up = false, false
		}
		if !port.Up {
			port.Speed = 0
		}
		p.Ports = append(p.Ports, port)
	}
	p.Ports = append(p.Ports, Port{Name: "Po1", Type: 161, Speed: 2000, Up: true, Admin: true, Alias: "nas-lag"})
	p.Extra = []Port{{Name: "vlan 1", Type: 136, Up: true, Admin: true}, {Name: "loopback1", Type: 24, Up: true, Admin: true}}
	return p
}

// Generic8 ist ein einfacher 8-Port-Switch ohne ENTITY-MIB mit Namen „Port 1…8“.
func Generic8() Profile {
	p := Profile{Descr: "Smart Switch GS308E", Name: "office-switch"}
	for n := 1; n <= 8; n++ {
		p.Ports = append(p.Ports, Port{Name: fmt.Sprintf("Port %d", n), Type: 6, Speed: 1000, Up: n <= 5, Admin: true})
	}
	return p
}

type value struct {
	typ gosnmp.Asn1BER
	v   any
}

// Agent ist der laufende Fake.
type Agent struct {
	conn      *net.UDPConn
	community string
	decoder   *gosnmp.GoSNMP

	mu       sync.Mutex
	values   map[string]value
	sorted   []string
	silent   bool
	requests int
	done     chan struct{}
}

// Start lauscht auf addr (UDP, „127.0.0.1:0“ = freier Port).
func Start(addr, community string, p Profile) (*Agent, error) {
	ua, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp", ua)
	if err != nil {
		return nil, err
	}
	a := &Agent{conn: conn, community: community, values: map[string]value{}, done: make(chan struct{}),
		decoder: &gosnmp.GoSNMP{Version: gosnmp.Version2c, Logger: gosnmp.NewLogger(log.New(io.Discard, "", 0))}}
	a.load(p)
	go a.serve()
	return a, nil
}

func (a *Agent) Addr() string { return a.conn.LocalAddr().String() }

func (a *Agent) Close() {
	a.conn.Close()
	<-a.done
}

// Requests liefert die Anzahl beantworteter Anfragen.
func (a *Agent) Requests() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.requests
}

// Silent: keine Antworten mehr (Gerät weg bzw. Timeout).
func (a *Agent) Silent(on bool) {
	a.mu.Lock()
	a.silent = on
	a.mu.Unlock()
}

func (a *Agent) set(oid string, typ gosnmp.Asn1BER, v any) {
	if _, ok := a.values[oid]; !ok {
		a.sorted = nil
	}
	a.values[oid] = value{typ, v}
}

func (a *Agent) load(p Profile) {
	a.set(sys+".1.0", gosnmp.OctetString, p.Descr)
	a.set(sys+".2.0", gosnmp.ObjectIdentifier, ".1.3.6.1.4.1.9.6.1.1300")
	a.set(sys+".3.0", gosnmp.TimeTicks, uint32(123456789))
	a.set(sys+".5.0", gosnmp.OctetString, p.Name)
	all := append(slices.Clone(p.Ports), p.Extra...)
	for n, port := range all {
		idx := strconv.Itoa(n + 1)
		if port.Type == 161 {
			idx = strconv.Itoa(1000 + n) // LAGs liegen bei vielen Geräten in eigenem Bereich
		}
		if n >= len(p.Ports) {
			idx = strconv.Itoa(100000 + n)
		}
		updown := func(b bool) int {
			if b {
				return 1
			}
			return 2
		}
		a.set(ifEntry+".1."+idx, gosnmp.Integer, mustAtoi(idx))
		a.set(ifEntry+".2."+idx, gosnmp.OctetString, port.Name)
		a.set(ifEntry+".3."+idx, gosnmp.Integer, port.Type)
		a.set(ifEntry+".7."+idx, gosnmp.Integer, updown(port.Admin))
		a.set(ifEntry+".8."+idx, gosnmp.Integer, updown(port.Up))
		for _, c := range []string{".13.", ".14.", ".19.", ".20."} {
			a.set(ifEntry+c+idx, gosnmp.Counter32, uint32(0))
		}
		a.set(ifX+".1."+idx, gosnmp.OctetString, port.Name)
		a.set(ifX+".6."+idx, gosnmp.Counter64, uint64(0))
		a.set(ifX+".10."+idx, gosnmp.Counter64, uint64(0))
		a.set(ifX+".15."+idx, gosnmp.Gauge32, port.Speed)
		a.set(ifX+".18."+idx, gosnmp.OctetString, port.Alias)
	}
	if !p.Entity {
		return
	}
	// Gehäuse (1) mit Ports (ab 100), Zuordnung zu ifIndex über entAliasMappingTable
	a.set(entPhys+".2.1", gosnmp.OctetString, p.Descr)
	a.set(entPhys+".4.1", gosnmp.Integer, 0)
	a.set(entPhys+".5.1", gosnmp.Integer, 3)
	a.set(entPhys+".6.1", gosnmp.Integer, -1)
	a.set(entPhys+".7.1", gosnmp.OctetString, "1")
	a.set(entPhys+".10.1", gosnmp.OctetString, p.Firmware)
	a.set(entPhys+".11.1", gosnmp.OctetString, p.Serial)
	a.set(entPhys+".13.1", gosnmp.OctetString, p.Model)
	for n, port := range p.Ports {
		if port.Type != 6 {
			continue
		}
		e := strconv.Itoa(100 + n)
		descr := "Gigabit Ethernet Port"
		if port.SFP {
			descr = "SFP Gigabit Ethernet Port"
		}
		a.set(entPhys+".2."+e, gosnmp.OctetString, descr)
		a.set(entPhys+".4."+e, gosnmp.Integer, 1)
		a.set(entPhys+".5."+e, gosnmp.Integer, 10)
		a.set(entPhys+".6."+e, gosnmp.Integer, n+1)
		a.set(entPhys+".7."+e, gosnmp.OctetString, port.Name)
		a.set(entMap+"."+e+".0", gosnmp.ObjectIdentifier, ".1.3.6.1.2.1.2.2.1.1."+strconv.Itoa(n+1))
	}
}

func mustAtoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// ifIndexOf sucht den ifIndex eines Ports nach Namen.
func (a *Agent) ifIndexOf(name string) string {
	for oid, v := range a.values {
		if idx, ok := strings.CutPrefix(oid, ifX+".1."); ok && v.v == name {
			return idx
		}
	}
	return ""
}

// AddTraffic zählt Bytes auf einen Port (Name wie im Profil).
func (a *Agent) AddTraffic(name string, in, out uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	idx := a.ifIndexOf(name)
	if idx == "" {
		return
	}
	a.values[ifX+".6."+idx] = value{gosnmp.Counter64, a.values[ifX+".6."+idx].v.(uint64) + in}
	a.values[ifX+".10."+idx] = value{gosnmp.Counter64, a.values[ifX+".10."+idx].v.(uint64) + out}
}

// SetCounters setzt die Byte-Zähler eines Ports (z. B. Neustart: 0).
func (a *Agent) SetCounters(name string, in, out uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if idx := a.ifIndexOf(name); idx != "" {
		a.values[ifX+".6."+idx] = value{gosnmp.Counter64, in}
		a.values[ifX+".10."+idx] = value{gosnmp.Counter64, out}
	}
}

// AddErrors zählt Eingangsfehler auf einen Port.
func (a *Agent) AddErrors(name string, n uint32) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if idx := a.ifIndexOf(name); idx != "" {
		a.values[ifEntry+".14."+idx] = value{gosnmp.Counter32, a.values[ifEntry+".14."+idx].v.(uint32) + n}
	}
}

// SetLink schaltet den Link eines Ports.
func (a *Agent) SetLink(name string, up bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if idx := a.ifIndexOf(name); idx != "" {
		st, speed := 2, uint32(0)
		if up {
			st, speed = 1, 1000
		}
		a.values[ifEntry+".8."+idx] = value{gosnmp.Integer, st}
		a.values[ifX+".15."+idx] = value{gosnmp.Gauge32, speed}
	}
}

// Ports liefert die Namen aller Ports mit Link (für den Mock-Traffic).
func (a *Agent) Ports() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []string
	for oid, v := range a.values {
		if idx, ok := strings.CutPrefix(oid, ifEntry+".8."); ok && v.v == 1 {
			if name, ok := a.values[ifX+".1."+idx].v.(string); ok {
				out = append(out, name)
			}
		}
	}
	slices.Sort(out)
	return out
}

func compareOID(x, y string) int {
	xs, ys := strings.Split(strings.TrimPrefix(x, "."), "."), strings.Split(strings.TrimPrefix(y, "."), ".")
	for n := 0; n < len(xs) && n < len(ys); n++ {
		if c := cmp.Compare(mustAtoi(xs[n]), mustAtoi(ys[n])); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(xs), len(ys))
}

// next liefert die erste OID nach oid (lexikografisch nach Zahlen).
func (a *Agent) next(oid string) (string, bool) {
	if a.sorted == nil {
		for k := range a.values {
			a.sorted = append(a.sorted, k)
		}
		slices.SortFunc(a.sorted, compareOID)
	}
	i, found := slices.BinarySearchFunc(a.sorted, oid, compareOID)
	if found {
		i++
	}
	if i >= len(a.sorted) {
		return "", false
	}
	return a.sorted[i], true
}

func (a *Agent) pdu(oid string) gosnmp.SnmpPDU {
	v := a.values[oid]
	return gosnmp.SnmpPDU{Name: oid, Type: v.typ, Value: v.v}
}

func (a *Agent) nextPDU(oid string) gosnmp.SnmpPDU {
	if n, ok := a.next(oid); ok {
		return a.pdu(n)
	}
	return gosnmp.SnmpPDU{Name: oid, Type: gosnmp.EndOfMibView}
}

func (a *Agent) serve() {
	defer close(a.done)
	buf := make([]byte, 65535)
	for {
		n, from, err := a.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if out := a.handle(buf[:n]); out != nil {
			a.conn.WriteToUDP(out, from)
		}
	}
}

func (a *Agent) handle(in []byte) []byte {
	req, err := a.decoder.SnmpDecodePacket(in)
	if err != nil || req.Version != gosnmp.Version2c {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.silent || req.Community != a.community {
		return nil
	}
	a.requests++
	var vars []gosnmp.SnmpPDU
	switch req.PDUType {
	case gosnmp.GetRequest:
		for _, v := range req.Variables {
			if _, ok := a.values[v.Name]; ok {
				vars = append(vars, a.pdu(v.Name))
			} else {
				vars = append(vars, gosnmp.SnmpPDU{Name: v.Name, Type: gosnmp.NoSuchObject})
			}
		}
	case gosnmp.GetNextRequest:
		for _, v := range req.Variables {
			vars = append(vars, a.nextPDU(v.Name))
		}
	case gosnmp.GetBulkRequest:
		nr := min(int(req.NonRepeaters), len(req.Variables))
		for _, v := range req.Variables[:nr] {
			vars = append(vars, a.nextPDU(v.Name))
		}
		rep := req.Variables[nr:]
		last := make([]string, len(rep))
		for k, v := range rep {
			last[k] = v.Name
		}
		for r := 0; r < int(min(req.MaxRepetitions, 50)) && len(rep) > 0; r++ {
			end := true
			for k := range rep {
				p := a.nextPDU(last[k])
				vars = append(vars, p)
				last[k] = p.Name
				end = end && p.Type == gosnmp.EndOfMibView
			}
			if end {
				break
			}
		}
	default:
		return nil
	}
	resp := &gosnmp.SnmpPacket{Version: gosnmp.Version2c, Community: req.Community, PDUType: gosnmp.GetResponse,
		RequestID: req.RequestID, Variables: vars, Logger: gosnmp.NewLogger(log.New(io.Discard, "", 0))}
	out, err := resp.MarshalMsg()
	if err != nil {
		return nil
	}
	return out
}
