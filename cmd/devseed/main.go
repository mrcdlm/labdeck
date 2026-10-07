// devseed schreibt einen erfundenen Verlauf für die Kennzahlen bestehender
// Integrationen in eine labdeck-Datenbank – für Entwicklung und Screenshots,
// damit Diagramme nicht erst nach Wochen etwas zeigen. Nicht Teil des Images.
//
//	go run ./cmd/devseed -db /pfad/app.db
//
// Für jede Integration mit Snapshot entstehen Tageswerte (1 Jahr) und
// Rohwerte (7 Tage, stündlich), die zum aktuellen Wert hin ansteigen.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"math"
	"math/rand/v2"
	"time"

	"github.com/mrcdlm/labdeck/internal/integrations"
	"github.com/mrcdlm/labdeck/internal/store"
)

func main() {
	path := flag.String("db", "data/app.db", "Pfad zur Datenbank")
	flag.Parse()
	ctx := context.Background()
	st, err := store.Open(ctx, *path)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()
	list, err := st.ListIntegrations(ctx)
	if err != nil {
		log.Fatal(err)
	}
	now := time.Now()
	for _, it := range list {
		var snap integrations.Snapshot
		if json.Unmarshal([]byte(it.Snapshot), &snap) != nil {
			continue
		}
		n := 0
		for _, m := range snap.Metrics {
			if m.Unit == "%" {
				continue
			}
			// Wert vor t Tagen: wächst über das Jahr auf den heutigen Wert
			at := func(days float64) float64 {
				v := m.Value * (1 - 0.35*days/365) * (1 + 0.01*math.Sin(days))
				if m.Unit == "" {
					v = math.Round(v)
				}
				return math.Max(v, 0)
			}
			for d := 365; d >= 1; d-- {
				day := now.AddDate(0, 0, -d)
				if err := st.SaveSample(ctx, it.ID, m.Key, day, at(float64(d))); err != nil {
					log.Fatal(err)
				}
			}
			for h := 7 * 24; h >= 1; h-- {
				t := now.Add(-time.Duration(h) * time.Hour)
				if err := st.SaveSample(ctx, it.ID, m.Key, t, at(float64(h)/24)*(1+0.002*rand.Float64())); err != nil {
					log.Fatal(err)
				}
			}
			n++
		}
		log.Printf("%s: Verlauf für %d Kennzahlen", it.Name, n)
	}
}
