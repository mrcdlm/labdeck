package snmp

import (
	"cmp"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/mrcdlm/labdeck/internal/integrations"
)

// Bauformen eines Ports im Frontbild.
const (
	mediaRJ45 = "rj45"
	mediaSFP  = "sfp"
	mediaLAG  = "lag"
)

// entPhysicalClass (ENTITY-MIB).
const (
	entChassis = 3
	entPort    = 10
)

type port struct {
	ifIndex, name, alias string
	typ                  uint64
	admin, oper          uint64
	speed                uint64 // Mbit/s
	inOctets, outOctets  uint64
	errs                 uint64 // Fehler + Discards in beide Richtungen
	hasCounters          bool

	media     string
	unit, pos int
	sfpHint   bool // ENTITY-MIB nennt SFP/Glasfaser

	inRate, outRate *float64 // Bytes/s; nil = noch keine Rate
	newErrs         *uint64  // seit der letzten Abfrage
}

// buildPorts wählt die physischen Ports (Ethernet, optional LAGs) aus der
// IF-MIB; VLANs, Loopback, CPU und nicht vorhandene Ports entfallen.
func buildPorts(d *device, lags bool) []*port {
	var out []*port
	for idx := range d.ifs[colIfType] {
		typ, _ := d.ifs.num(colIfType, idx)
		oper, _ := d.ifs.num(colIfOperStatus, idx)
		if oper == ifNotPresent {
			continue
		}
		switch {
		case typ == ifTypeEther, typ == ifTypeGigE:
		case typ == ifTypeLAG && lags:
		default:
			continue
		}
		p := &port{ifIndex: idx, typ: typ, oper: oper}
		p.admin, _ = d.ifs.num(colIfAdminStatus, idx)
		p.name = strings.TrimSpace(cmp.Or(d.ifs.str(colIfName, idx), d.ifs.str(colIfDescr, idx), "if"+idx))
		p.alias = strings.TrimSpace(d.ifs.str(colIfAlias, idx))
		p.speed, _ = d.ifs.num(colIfHighSpeed, idx)
		var okIn, okOut bool
		p.inOctets, okIn = d.ifs.num(colIfHCInOctets, idx)
		p.outOctets, okOut = d.ifs.num(colIfHCOutOctets, idx)
		p.hasCounters = okIn && okOut
		for _, c := range []string{colIfInErrors, colIfOutErrors, colIfInDiscards, colIfOutDiscards} {
			n, _ := d.ifs.num(c, idx)
			p.errs += n
		}
		if typ == ifTypeLAG {
			p.media = mediaLAG
		}
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b *port) int { return compareIndex(a.ifIndex, b.ifIndex) })
	return out
}

// namePattern zerlegt Portnamen wie „gi1/0/24“, „GigabitEthernet0/3“,
// „TenGigabitEthernet1/1/2“, „eth5“, „Port 12“ in Präfix und Nummern.
var namePattern = regexp.MustCompile(`^([A-Za-z][A-Za-z\- ]*?)\s*((?:\d+/)*\d+)$`)

// Präfixe für SFP-Käfige und LAGs (klein geschrieben, Anfang des Namens).
var (
	sfpPrefixes = []string{"te", "tengig", "xe", "xg", "twe", "twentyfivegig", "fo", "fortygig", "hu", "hundredgig", "sfp", "qsfp"}
	lagPrefixes = []string{"po", "port-channel", "lag", "bond", "ae", "trk", "ch"}
)

func splitName(name string) (prefix string, nums []int, ok bool) {
	m := namePattern.FindStringSubmatch(strings.TrimSpace(name))
	if m == nil {
		return "", nil, false
	}
	for _, s := range strings.Split(m[2], "/") {
		n, _ := strconv.Atoi(s)
		nums = append(nums, n)
	}
	return strings.ToLower(strings.TrimSpace(m[1])), nums, true
}

func hasPrefix(prefix string, list []string) bool {
	for _, p := range list {
		// „te“ passt auf „tengigabitethernet“, aber „po“ nicht auf „port 12“
		if prefix == p || strings.HasPrefix(prefix, p) && !strings.HasPrefix(prefix, "port") {
			return true
		}
	}
	return false
}

// modelUplinks erkennt die Uplinks aus Modellnamen wie „C1300-24P-4G“,
// „CBS350-48FP-4X“ (48 Ports + 4 SFP) – Schema vieler Hersteller.
var modelUplinks = regexp.MustCompile(`-(\d+)[A-Z]*-(\d+)[GXS]\b`)

// front legt Bauform, Einheit und Position jedes Ports fest:
//  1. Einheit/Position aus dem Namen (Stack/Slot/Port), sonst Reihenfolge
//  2. Bauform aus ENTITY-MIB, Namenspräfix, Modellname (in dieser Reihenfolge)
//  3. Frontlayout aus dem Formular überschreibt die Bauform
func front(ports []*port, d *device, layout string) {
	entityHints(ports, d)
	for n, p := range ports {
		p.unit, p.pos = 1, n+1
		prefix, nums, ok := splitName(p.name)
		if ok {
			p.pos = nums[len(nums)-1]
			if len(nums) >= 2 {
				p.unit = nums[0]
			}
		}
		if p.media == mediaLAG {
			continue
		}
		switch {
		case ok && hasPrefix(prefix, lagPrefixes):
			p.media = mediaLAG
		case p.sfpHint, ok && hasPrefix(prefix, sfpPrefixes):
			p.media = mediaSFP
		default:
			p.media = mediaRJ45
		}
	}
	phys := slices.DeleteFunc(slices.Clone(ports), func(p *port) bool { return p.media == mediaLAG })
	slices.SortStableFunc(phys, func(a, b *port) int { return cmp.Or(cmp.Compare(a.unit, b.unit), cmp.Compare(a.pos, b.pos)) })

	// Modellname: die letzten Ports sind Uplinks (nur wenn sonst nichts SFP ist)
	if !slices.ContainsFunc(phys, func(p *port) bool { return p.media == mediaSFP }) {
		model, _, _ := chassisInfo(d)
		if m := modelUplinks.FindStringSubmatch(cmp.Or(model, d.descr)); m != nil {
			base, _ := strconv.Atoi(m[1])
			up, _ := strconv.Atoi(m[2])
			if up > 0 && len(phys) == base+up {
				for _, p := range phys[base:] {
					p.media = mediaSFP
				}
			}
		}
	}

	if groups := parseLayout(layout); groups != nil {
		n := 0
		for _, g := range groups {
			for k := 0; k < g.count && n < len(phys); k++ {
				phys[n].media = g.media
				n++
			}
		}
	}
}

// entityHints liest aus der ENTITY-MIB, welche Ports SFP-Käfige sind.
func entityHints(ports []*port, d *device) {
	if d.ent == nil {
		return
	}
	byIf := map[string]*port{}
	byName := map[string]*port{}
	for _, p := range ports {
		byIf[p.ifIndex] = p
		byName[shortName(p.name)] = p
	}
	for idx := range d.ent[colEntClass] {
		if c, _ := d.ent.num(colEntClass, idx); c != entPort {
			continue
		}
		p := byIf[d.alias[idx]]
		if p == nil {
			p = byName[shortName(d.ent.str(colEntName, idx))]
		}
		if p == nil {
			continue
		}
		text := strings.ToLower(d.ent.str(colEntDescr, idx) + " " + d.ent.str(colEntName, idx))
		if strings.Contains(text, "combo") {
			continue // Combo-Port: RJ45 und SFP – als RJ45 zeichnen
		}
		for _, w := range []string{"sfp", "fiber", "fibre", "gbic", "xfp", "optical"} {
			if strings.Contains(text, w) {
				p.sfpHint = true
				break
			}
		}
	}
}

// shortName vereinheitlicht Portnamen: „GigabitEthernet1/0/1“ → „gi1/0/1“.
func shortName(name string) string {
	prefix, nums, ok := splitName(name)
	if !ok {
		return strings.ToLower(strings.TrimSpace(name))
	}
	switch {
	case prefix == "port-channel":
		prefix = "po"
	case strings.HasPrefix(prefix, "port"): // „Port 12“ ≠ „Po12“
		prefix = "port"
	case len(prefix) > 2:
		prefix = prefix[:2]
	}
	parts := make([]string, len(nums))
	for n, x := range nums {
		parts[n] = strconv.Itoa(x)
	}
	return prefix + strings.Join(parts, "/")
}

type layoutGroup struct {
	count int
	media string
}

var layoutItem = regexp.MustCompile(`^(\d+)\s*x?\s*(rj45|rj-45|sfp\+?|sfp28|copper|kupfer|fiber|glas)$`)

// parseLayout liest „24 rj45, 4 sfp“; ungültig oder leer = nil.
func parseLayout(s string) []layoutGroup {
	var out []layoutGroup
	for _, item := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return r == ',' || r == ';' || r == '+' }) {
		m := layoutItem.FindStringSubmatch(strings.TrimSpace(item))
		if m == nil {
			return nil
		}
		n, _ := strconv.Atoi(m[1])
		media := mediaRJ45
		if strings.HasPrefix(m[2], "sfp") || m[2] == "fiber" || m[2] == "glas" {
			media = mediaSFP
		}
		out = append(out, layoutGroup{n, media})
	}
	return out
}

// monitorSet: überwachte Ports laut Einstellung.
func monitorSet(ports []*port, cfg integrations.Config) map[*port]bool {
	out := map[*port]bool{}
	switch cfg.String("monitor") {
	case "none":
	case "list":
		sel := parsePortList(cfg.String("monitor_ports"))
		for _, p := range ports {
			if sel.match(p) {
				out[p] = true
			}
		}
	default: // described
		for _, p := range ports {
			if p.alias != "" {
				out[p] = true
			}
		}
	}
	return out
}

type portList struct {
	names map[string]bool // kurze Namen, z. B. gi1/0/3
	nums  map[int]bool    // Portnummern (auf jeder Einheit)
}

func (l portList) match(p *port) bool {
	return l.names[shortName(p.name)] || l.nums[p.pos] && p.media != mediaLAG
}

var rangeItem = regexp.MustCompile(`^(.*?)(\d+)\s*-\s*(\d+)$`)

// parsePortList liest „gi1/0/1-8, gi1/0/24, 5, 10-12“.
func parsePortList(s string) portList {
	l := portList{names: map[string]bool{}, nums: map[int]bool{}}
	for _, item := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == ' ' }) {
		prefix, from, to := item, -1, -1
		if m := rangeItem.FindStringSubmatch(item); m != nil {
			prefix = m[1]
			from, _ = strconv.Atoi(m[2])
			to, _ = strconv.Atoi(m[3])
			if to < from || to-from > 1000 {
				continue
			}
		}
		if from < 0 {
			if n, err := strconv.Atoi(item); err == nil {
				l.nums[n] = true
			} else {
				l.names[shortName(item)] = true
			}
			continue
		}
		for n := from; n <= to; n++ {
			if prefix == "" {
				l.nums[n] = true
			} else {
				l.names[shortName(prefix+strconv.Itoa(n))] = true
			}
		}
	}
	return l
}
