// Package live hält die Kennzahlen der letzten Stunde in voller
// Abfrageauflösung im Speicher. In der Datenbank landen Rohwerte höchstens
// einmal je Minute; Live-Kacheln brauchen aber jeden Wert (Intervall ab 5 s).
// Nach einem Neustart ist der Puffer leer – die API ergänzt dann aus der DB.
package live

import (
	"sync"
	"time"
)

// Window ist die Zeitspanne, die der Puffer vorhält.
const Window = time.Hour

// maxPoints begrenzt den Speicher je Kennzahl (1 h bei 5 s = 720).
const maxPoints = 800

// Point ist ein Wert zum Zeitpunkt At.
type Point struct {
	At    time.Time
	Value float64
}

// Buffer: Integration → Kennzahl → Werte, aufsteigend nach Zeit.
type Buffer struct {
	mu   sync.Mutex
	data map[int64]map[string][]Point
}

func New() *Buffer { return &Buffer{data: map[int64]map[string][]Point{}} }

// Add speichert die Werte einer Abfrage und verwirft Werte älter als Window.
func (b *Buffer) Add(id int64, at time.Time, values map[string]float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	m := b.data[id]
	if m == nil {
		m = map[string][]Point{}
		b.data[id] = m
	}
	cut := at.Add(-Window)
	for key, v := range values {
		list := append(m[key], Point{at, v})
		n := 0
		for n < len(list) && (list[n].At.Before(cut) || len(list)-n > maxPoints) {
			n++
		}
		m[key] = list[n:]
	}
	// Kennzahlen, die nicht mehr geliefert werden, laufen aus
	for key, list := range m {
		if _, ok := values[key]; !ok && (len(list) == 0 || list[len(list)-1].At.Before(cut)) {
			delete(m, key)
		}
	}
}

// Points liefert die Werte einer Kennzahl ab from (Kopie).
func (b *Buffer) Points(id int64, key string, from time.Time) []Point {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []Point
	for _, p := range b.data[id][key] {
		if !p.At.Before(from) {
			out = append(out, p)
		}
	}
	return out
}

// Drop entfernt eine Integration (gelöscht).
func (b *Buffer) Drop(id int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.data, id)
}
