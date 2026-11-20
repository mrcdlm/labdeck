package snmp

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gosnmp/gosnmp"
)

// Verkabelung und VLANs für den Netzplan – ebenfalls nur Standard-MIBs:
// LLDP-MIB (IEEE 802.1AB) für die Nachbarn je Port, BRIDGE-MIB (RFC 4188) und
// Q-BRIDGE-MIB (RFC 4363) für die VLANs je Port. Beide sind optional: fehlen
// sie (oder antwortet der Switch dort nicht), bleibt der Port ohne Angaben.
const (
	oidLldpLocPort = ".1.0.8802.1.1.2.1.3.7.1" // lldpLocPortTable
	oidLldpRem     = ".1.0.8802.1.1.2.1.4.1.1" // lldpRemTable

	colLldpLocPortID   = oidLldpLocPort + ".3"
	colLldpLocPortDesc = oidLldpLocPort + ".4"

	colLldpRemChassisSub = oidLldpRem + ".4"
	colLldpRemChassisID  = oidLldpRem + ".5"
	colLldpRemPortSub    = oidLldpRem + ".6"
	colLldpRemPortID     = oidLldpRem + ".7"
	colLldpRemPortDesc   = oidLldpRem + ".8"
	colLldpRemSysName    = oidLldpRem + ".9"

	colBasePortIfIndex   = ".1.3.6.1.2.1.17.1.4.1.2"     // dot1dBasePortIfIndex
	colPvid              = ".1.3.6.1.2.1.17.7.1.4.5.1.1" // dot1qPvid
	colVlanStaticName    = ".1.3.6.1.2.1.17.7.1.4.3.1.1" // dot1qVlanStaticName
	colVlanStaticEgress  = ".1.3.6.1.2.1.17.7.1.4.3.1.2" // dot1qVlanStaticEgressPorts
	colVlanStaticUntaged = ".1.3.6.1.2.1.17.7.1.4.3.1.4" // dot1qVlanStaticUntaggedPorts

	// topoTTL: Verkabelung und VLANs ändern sich selten
	topoTTL = 5 * time.Minute
)

var lldpColumns = []string{colLldpLocPortID, colLldpLocPortDesc, colLldpRemChassisSub, colLldpRemChassisID,
	colLldpRemPortSub, colLldpRemPortID, colLldpRemPortDesc, colLldpRemSysName}

var vlanColumns = []string{colBasePortIfIndex, colPvid, colVlanStaticName, colVlanStaticEgress, colVlanStaticUntaged}

// neighbor ist ein LLDP-Nachbar an einem Port.
type neighbor struct {
	name, port, portDesc, chassis string
}

// portVlans: VLAN ohne Tag (PVID) und getaggte VLANs eines Ports.
type portVlans struct {
	pvid   int
	tagged []int
}

// topo: Nachbarn und VLANs je ifIndex, Namen der VLANs.
type topo struct {
	neighbors map[string][]neighbor
	vlans     map[string]*portVlans
	vlanNames map[int]string
}

// fetchTopo liest LLDP- und Q-BRIDGE-MIB. Fehler sind kein Fehler der
// Abfrage – dann fehlen nur Kabel bzw. VLANs.
func fetchTopo(ctx context.Context, g *gosnmp.GoSNMP, ifs table) *topo {
	t := &topo{neighbors: map[string][]neighbor{}, vlans: map[string]*portVlans{}, vlanNames: map[int]string{}}
	if lldp, err := walk(ctx, g, lldpColumns); err == nil {
		t.neighbors = parseLLDP(lldp, ifs)
	}
	if ctx.Err() != nil {
		return t
	}
	if q, err := walk(ctx, g, vlanColumns); err == nil {
		t.vlans, t.vlanNames = parseVlans(q)
	}
	return t
}

// parseLLDP ordnet die Nachbarn den Ports zu. lldpRemTable ist nach
// TimeMark.LocalPortNum.Index indiziert; LocalPortNum ist meist der ifIndex,
// sicher ist aber nur der Name aus lldpLocPortTable.
func parseLLDP(t table, ifs table) map[string][]neighbor {
	byName := map[string]string{}
	for idx := range ifs[colIfDescr] {
		for _, col := range []string{colIfName, colIfDescr} {
			if n := strings.ToLower(strings.TrimSpace(ifs.str(col, idx))); n != "" {
				if _, ok := byName[n]; !ok {
					byName[n] = idx
				}
			}
		}
	}
	ifIndexOf := func(local string) string {
		for _, col := range []string{colLldpLocPortDesc, colLldpLocPortID} {
			if idx, ok := byName[strings.ToLower(strings.TrimSpace(t.str(col, local)))]; ok {
				return idx
			}
		}
		if _, ok := ifs[colIfDescr][local]; ok {
			return local
		}
		return ""
	}
	out := map[string][]neighbor{}
	idxs := make([]string, 0, len(t[colLldpRemSysName]))
	for idx := range t[colLldpRemChassisID] {
		idxs = append(idxs, idx)
	}
	slices.SortFunc(idxs, compareIndex)
	for _, idx := range idxs {
		parts := strings.Split(idx, ".")
		if len(parts) != 3 {
			continue
		}
		port := ifIndexOf(parts[1])
		if port == "" {
			continue
		}
		chassisSub, _ := t.num(colLldpRemChassisSub, idx)
		portSub, _ := t.num(colLldpRemPortSub, idx)
		n := neighbor{
			name:     strings.TrimSpace(t.str(colLldpRemSysName, idx)),
			port:     lldpID(t[colLldpRemPortID][idx], portSub == 3),
			portDesc: strings.TrimSpace(t.str(colLldpRemPortDesc, idx)),
			chassis:  lldpID(t[colLldpRemChassisID][idx], chassisSub == 4),
		}
		if n.name == "" && n.chassis == "" {
			continue
		}
		out[port] = append(out[port], n)
	}
	return out
}

// lldpID macht aus einer Chassis-/Port-ID Text: MAC-Adressen (Subtyp oder
// nicht druckbar) als aa:bb:cc:dd:ee:ff, sonst den Text.
func lldpID(v any, mac bool) string {
	var b []byte
	switch x := v.(type) {
	case []byte:
		b = x
	case string:
		b = []byte(x)
	default:
		return ""
	}
	printable := len(b) > 0
	for _, r := range string(b) {
		if !unicode.IsPrint(r) {
			printable = false
			break
		}
	}
	if (mac || !printable) && len(b) == 6 {
		parts := make([]string, len(b))
		for i, c := range b {
			parts[i] = fmt.Sprintf("%02x", c)
		}
		return strings.Join(parts, ":")
	}
	if !printable {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// parseVlans: PVID je Port und getaggte VLANs (Egress ohne Untagged) aus den
// statischen VLANs; Bridge-Ports werden über dot1dBasePortIfIndex zum ifIndex.
func parseVlans(t table) (map[string]*portVlans, map[int]string) {
	ifIndex := map[int]string{}
	for bp := range t[colBasePortIfIndex] {
		if v, ok := t.num(colBasePortIfIndex, bp); ok {
			if n, err := strconv.Atoi(bp); err == nil {
				ifIndex[n] = strconv.FormatUint(v, 10)
			}
		}
	}
	out := map[string]*portVlans{}
	get := func(bp int) *portVlans {
		idx, ok := ifIndex[bp]
		if !ok {
			return nil
		}
		if out[idx] == nil {
			out[idx] = &portVlans{}
		}
		return out[idx]
	}
	for bp := range t[colPvid] {
		n, err := strconv.Atoi(bp)
		v, ok := t.num(colPvid, bp)
		if err != nil || !ok {
			continue
		}
		if p := get(n); p != nil {
			p.pvid = int(v)
		}
	}
	names := map[int]string{}
	for vid := range t[colVlanStaticEgress] {
		id, err := strconv.Atoi(vid)
		if err != nil {
			continue
		}
		names[id] = strings.TrimSpace(t.str(colVlanStaticName, vid))
		untagged := portSet(t[colVlanStaticUntaged][vid])
		for _, bp := range portSet(t[colVlanStaticEgress][vid]) {
			if slices.Contains(untagged, bp) {
				continue
			}
			if p := get(bp); p != nil {
				p.tagged = append(p.tagged, id)
			}
		}
	}
	for _, p := range out {
		slices.Sort(p.tagged)
	}
	return out, names
}

// portSet liest eine PortList (RFC 4363): Bit 7 des ersten Bytes ist Port 1.
func portSet(v any) []int {
	b, _ := v.([]byte)
	if s, ok := v.(string); ok {
		b = []byte(s)
	}
	var out []int
	for i, c := range b {
		for bit := 0; bit < 8; bit++ {
			if c&(0x80>>bit) != 0 {
				out = append(out, i*8+bit+1)
			}
		}
	}
	return out
}

// vlanList: „10,20,30“.
func vlanList(ids []int) string {
	s := make([]string, len(ids))
	for i, id := range ids {
		s[i] = strconv.Itoa(id)
	}
	return strings.Join(s, ",")
}
