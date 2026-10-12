package main

import (
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/mrcdlm/labdeck/internal/integrations/snmp/snmptest"
)

// mountSNMP startet zwei SNMP-Agenten: Cisco C1300 auf addr und einen
// 8-Port-Switch ohne ENTITY-MIB auf dem nächsten Port. Beide erzeugen
// laufend Traffic, damit Raten und Live-Kacheln etwas zeigen.
func mountSNMP(mux *http.ServeMux, addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	p, _ := strconv.Atoi(port)
	c1300, err := snmptest.Start(addr, "labdeck-ro", snmptest.C1300())
	if err != nil {
		return err
	}
	small, err := snmptest.Start(net.JoinHostPort(host, strconv.Itoa(p+1)), "labdeck-ro", snmptest.Generic8())
	if err != nil {
		return err
	}
	go func() {
		// je Port eigener Grundpegel (Bytes/s), darauf Schwankungen
		base := map[string]float64{"gi1/0/1": 40e6, "gi1/0/2": 12e6, "gi1/0/3": 90e6, "gi1/0/24": 25e6, "Port 1": 5e6}
		for range time.Tick(time.Second) {
			for _, a := range []*snmptest.Agent{c1300, small} {
				for _, name := range a.Ports() {
					b := base[name]
					if b == 0 {
						b = 300e3
					}
					a.AddTraffic(name, uint64(b*(0.5+rand.Float64())), uint64(b*0.4*(0.5+rand.Float64()))) //nolint:gosec // Testdaten
				}
			}
		}
	}()
	mux.HandleFunc("POST /_snmp/link", func(w http.ResponseWriter, r *http.Request) {
		c1300.SetLink(r.URL.Query().Get("name"), r.URL.Query().Get("up") == "1")
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /_snmp/silent", func(w http.ResponseWriter, r *http.Request) {
		on := r.URL.Query().Get("on") == "1"
		c1300.Silent(on)
		small.Silent(on)
		w.WriteHeader(http.StatusNoContent)
	})
	return nil
}
