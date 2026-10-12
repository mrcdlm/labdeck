package live

import (
	"testing"
	"time"
)

func TestBuffer(t *testing.T) {
	b := New()
	t0 := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	for n := range 900 { // 75 min alle 5 s
		b.Add(1, t0.Add(time.Duration(n)*5*time.Second), map[string]float64{"cpu": float64(n)})
	}
	now := t0.Add(899 * 5 * time.Second)
	all := b.Points(1, "cpu", time.Time{})
	if len(all) != 721 || !all[0].At.Equal(now.Add(-Window)) || all[720].Value != 899 {
		t.Fatalf("Fenster: %d Punkte, erster %v", len(all), all[0].At)
	}
	if got := b.Points(1, "cpu", now.Add(-time.Minute)); len(got) != 13 {
		t.Errorf("letzte Minute: %d", len(got))
	}
	if b.Points(2, "cpu", time.Time{}) != nil || b.Points(1, "ram", time.Time{}) != nil {
		t.Error("unbekannt muss leer sein")
	}

	// Kennzahl fällt weg: läuft nach Window aus
	b.Add(1, now.Add(time.Second), map[string]float64{"ram": 1})
	if len(b.Points(1, "cpu", time.Time{})) == 0 {
		t.Error("cpu zu früh entfernt")
	}
	b.Add(1, now.Add(Window+2*time.Second), map[string]float64{"ram": 2})
	if len(b.Points(1, "cpu", time.Time{})) != 0 {
		t.Error("cpu nicht ausgelaufen")
	}

	b.Drop(1)
	if b.Points(1, "ram", time.Time{}) != nil {
		t.Error("Drop")
	}
}
