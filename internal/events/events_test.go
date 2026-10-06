package events

import "testing"

func TestBroker(t *testing.T) {
	b := NewBroker()
	a, unsubA := b.Subscribe()
	c, _ := b.Subscribe()

	b.Publish(TopicSettings)
	if got := <-a; got != TopicSettings {
		t.Fatalf("a: %q", got)
	}
	if got := <-c; got != TopicSettings {
		t.Fatalf("c: %q", got)
	}

	unsubA()
	unsubA() // doppelt abmelden ist harmlos
	if _, ok := <-a; ok {
		t.Fatal("a sollte geschlossen sein")
	}
	if b.Subscribers() != 1 {
		t.Fatalf("subs = %d", b.Subscribers())
	}

	// voller Puffer blockiert nicht
	for range 100 {
		b.Publish(TopicSettings)
	}

	b.Close()
	for range c { // leert und endet, weil geschlossen
	}
	if d, _ := b.Subscribe(); d != nil {
		if _, ok := <-d; ok {
			t.Fatal("Abo nach Close sollte geschlossen sein")
		}
	}
	Publish(nil, TopicSettings) // nil-Publisher ist erlaubt
}
