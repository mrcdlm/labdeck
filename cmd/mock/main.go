// mock startet nachgebaute APIs der unterstützten Dienste für die lokale
// Entwicklung und die Browser-Tests, damit labdeck ohne echte Dienste und
// Zugangsdaten läuft. Nicht Teil des Docker-Images.
//
//	go run ./cmd/mock -addr :8790
//	APP_PASSWORD=test go run ./cmd/server
//
// Dienste (Basis-URL in labdeck eintragen):
//
//	Healthchecks.io   http://localhost:8790/healthchecks   API-Key: hc-readonly (Read-only) oder hc-readwrite
//
// Steuerung (ohne Key):
//
//	POST /_healthchecks/fail?status=500             Ausfälle simulieren (status=0 beendet)
//	POST /_healthchecks/broken?on=1                 kaputtes JSON liefern (on=0 beendet)
//	POST /_healthchecks/delay?ms=3000               Antworten verzögern (ms=0 beendet)
//	POST /_healthchecks/status?name=DynDNS&status=down   Status eines Checks setzen
//	GET  /_healthchecks/requests                    Anzahl der API-Anfragen
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/mrcdlm/labdeck/internal/integrations/healthchecks/hctest"
)

func main() {
	addr := flag.String("addr", ":8790", "Listen-Adresse")
	hcRO := flag.String("hc-key", "hc-readonly", "Read-only-API-Key für Healthchecks")
	hcRW := flag.String("hc-rw-key", "hc-readwrite", "Read-write-API-Key für Healthchecks")
	flag.Parse()

	hc := hctest.New(*hcRO, *hcRW, hctest.Demo()...)

	mux := http.NewServeMux()
	mux.Handle("/healthchecks/", http.StripPrefix("/healthchecks", hc))

	intParam := func(r *http.Request, name string) int {
		n, _ := strconv.Atoi(r.URL.Query().Get(name))
		return n
	}
	mux.HandleFunc("POST /_healthchecks/fail", func(w http.ResponseWriter, r *http.Request) {
		hc.Fail(intParam(r, "status"))
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /_healthchecks/broken", func(w http.ResponseWriter, r *http.Request) {
		hc.Broken(r.URL.Query().Get("on") == "1")
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /_healthchecks/delay", func(w http.ResponseWriter, r *http.Request) {
		hc.Delay(time.Duration(intParam(r, "ms")) * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /_healthchecks/status", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch q.Get("status") {
		case "new", "up", "grace", "down", "paused":
		default:
			http.Error(w, "status muss new|up|grace|down|paused sein", http.StatusBadRequest)
			return
		}
		if !hc.SetStatus(q.Get("name"), q.Get("status")) {
			http.Error(w, "Check unbekannt", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /_healthchecks/requests", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, hc.Requests())
	})
	mux.HandleFunc("GET /_health", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })

	log.Printf("Mock auf %s – Healthchecks: http://localhost%s/healthchecks (Keys %s / %s)", *addr, *addr, *hcRO, *hcRW)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
