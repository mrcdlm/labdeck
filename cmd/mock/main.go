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
//	Immich            http://localhost:8790/immich         API-Key: immich-admin (oder immich-user: ohne Berechtigung)
//	Prometheus        http://localhost:8790/prometheus     ohne Anmeldung
//	HTTP-JSON         http://localhost:8790/json/stats     Header: Authorization: Token json-token
//	Proxmox Backup    http://localhost:8790/pbs            Token-ID: labdeck@pbs!dashboard, Secret: pbs-secret
//
// Steuerung (ohne Key), {svc} = healthchecks | immich | prometheus | json | pbs:
//
//	POST /_{svc}/fail?status=500                    Ausfälle simulieren (status=0 beendet)
//	POST /_{svc}/broken?on=1                        kaputtes JSON liefern (on=0 beendet)
//	POST /_{svc}/delay?ms=3000                      Antworten verzögern (ms=0 beendet)
//	GET  /_{svc}/requests                           Anzahl der API-Anfragen
//	POST /_healthchecks/status?name=DynDNS&status=down   Status eines Checks setzen
//	POST /_immich/photos?n=100                      Fotos hinzufügen (Verlauf)
//
// Webhook-Empfänger: beliebige Anfragen an /hook/… werden aufgezeichnet,
// GET /_hooks liefert sie, DELETE /_hooks leert die Liste.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/mrcdlm/labdeck/internal/integrations/healthchecks/hctest"
	"github.com/mrcdlm/labdeck/internal/integrations/immich/immichtest"
	"github.com/mrcdlm/labdeck/internal/integrations/pbs/pbstest"
	"github.com/mrcdlm/labdeck/internal/integrations/prometheus/promtest"
)

// controllable sind die gemeinsamen Steuerfunktionen der Fakes.
type controllable interface {
	http.Handler
	Fail(status int)
	Broken(on bool)
	Delay(d time.Duration)
	Requests() int
}

func intParam(r *http.Request, name string) int {
	n, _ := strconv.Atoi(r.URL.Query().Get(name))
	return n
}

// mount hängt einen Fake unter /{name}/ ein und seine Steuerung unter /_{name}/.
func mount(mux *http.ServeMux, name string, f controllable) {
	mux.Handle("/"+name+"/", http.StripPrefix("/"+name, f))
	mux.HandleFunc("POST /_"+name+"/fail", func(w http.ResponseWriter, r *http.Request) {
		f.Fail(intParam(r, "status"))
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /_"+name+"/broken", func(w http.ResponseWriter, r *http.Request) {
		f.Broken(r.URL.Query().Get("on") == "1")
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /_"+name+"/delay", func(w http.ResponseWriter, r *http.Request) {
		f.Delay(time.Duration(intParam(r, "ms")) * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /_"+name+"/requests", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, f.Requests())
	})
}

func main() {
	addr := flag.String("addr", ":8790", "Listen-Adresse")
	flag.Parse()

	mux := http.NewServeMux()

	hc := hctest.New("hc-readonly", "hc-readwrite", hctest.Demo()...)
	mount(mux, "healthchecks", hc)
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

	im := immichtest.New("immich-admin", "immich-user", immichtest.Demo()...)
	mount(mux, "immich", im)
	mux.HandleFunc("POST /_immich/photos", func(w http.ResponseWriter, r *http.Request) {
		im.AddPhotos(int64(intParam(r, "n")))
		w.WriteHeader(http.StatusNoContent)
	})

	mount(mux, "prometheus", promtest.New("", "", "").Demo(time.Now()))
	mount(mux, "json", &jsonStatus{})
	mount(mux, "pbs", pbstest.New("labdeck@pbs!dashboard:pbs-secret", pbstest.Demo()...))

	(&hookRecorder{}).mount(mux)

	mux.HandleFunc("GET /_health", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })

	log.Printf("Mock auf %s – Healthchecks: /healthchecks (hc-readonly), Immich: /immich (immich-admin), Prometheus: /prometheus, JSON: /json/stats, PBS: /pbs", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
