package api

import (
	"bufio"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mrcdlm/labdeck/internal/events"
)

// readEvent liest bis zur nächsten Leerzeile und liefert event- und data-Feld.
func readEvent(t *testing.T, rd *bufio.Reader) (event, data string) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			line, err := rd.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\n")
			switch {
			case line == "":
				if event != "" || data != "" {
					return
				}
			case strings.HasPrefix(line, "event: "):
				event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				data = strings.TrimPrefix(line, "data: ")
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("kein Ereignis erhalten")
	}
	return event, data
}

func openStream(t *testing.T, e *testEnv, c *http.Cookie) *bufio.Reader {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, _ := http.NewRequestWithContext(ctx, "GET", e.srv.URL+"/api/events", nil)
	req.AddCookie(c)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("status %d, type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	// auf Anmeldung beim Broker warten
	for i := 0; e.broker.Subscribers() == 0 && i < 100; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	return bufio.NewReader(resp.Body)
}

func TestEventsStream(t *testing.T) {
	e := newTestEnv(t, nil)
	c := e.login(t)
	rd := openStream(t, e, c)

	e.broker.Publish(events.TopicSettings)
	if ev, data := readEvent(t, rd); ev != "change" || data != "settings" {
		t.Fatalf("event=%q data=%q", ev, data)
	}
}

func TestEventsSessionExpiry(t *testing.T) {
	old := heartbeatInterval
	heartbeatInterval = 20 * time.Millisecond
	t.Cleanup(func() { heartbeatInterval = old })

	e := newTestEnv(t, nil)
	c := e.login(t)
	rd := openStream(t, e, c)
	e.do(t, "POST", "/api/logout", "", c)

	if ev, data := readEvent(t, rd); ev != "session" || data != "expired" {
		t.Fatalf("event=%q data=%q", ev, data)
	}
}
