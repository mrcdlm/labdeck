// Package nuttest ist ein kleiner upsd (NUT-Netzwerkprotokoll über TCP) für
// Tests und den Entwicklungs-Mock.
package nuttest

import (
	"bufio"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// UPS ist eine nachgebildete USV mit ihren Variablen.
type UPS struct {
	Name, Desc string
	Vars       map[string]string
}

type Server struct {
	ln         net.Listener
	mu         sync.Mutex
	ups        []UPS
	user, pass string // Anmeldung nötig für LIST VAR (leer = nein)
	stale      bool
	delay      time.Duration
	broken     bool
}

// Demo: eine USV im Netzbetrieb.
func Demo() UPS {
	return UPS{Name: "ups", Desc: "Eaton 5SC 1000", Vars: map[string]string{
		"ups.status": "OL CHRG", "battery.charge": "97", "battery.runtime": "1830", "ups.load": "31",
		"input.voltage": "231.0", "ups.mfr": "EATON", "ups.model": "5SC 1000", "ups.realpower.nominal": "700",
	}}
}

// Start lauscht auf addr ("127.0.0.1:0" = freier Port).
func Start(addr string, ups ...UPS) (*Server, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	s := &Server{ln: ln, ups: ups}
	go s.serve()
	return s, nil
}

func (s *Server) Addr() string { return s.ln.Addr().String() }
func (s *Server) Close()       { s.ln.Close() }

// RequireLogin verlangt Benutzer und Passwort.
func (s *Server) RequireLogin(user, pass string) {
	s.mu.Lock()
	s.user, s.pass = user, pass
	s.mu.Unlock()
}

// Set setzt eine Variable einer USV (z. B. ups.status auf "OB DISCHRG").
func (s *Server) Set(ups, key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.ups {
		if u.Name == ups {
			u.Vars[key] = value
		}
	}
}

func (s *Server) Stale(b bool)          { s.mu.Lock(); s.stale = b; s.mu.Unlock() }
func (s *Server) Delay(d time.Duration) { s.mu.Lock(); s.delay = d; s.mu.Unlock() }
func (s *Server) Broken(b bool)         { s.mu.Lock(); s.broken = b; s.mu.Unlock() }

func (s *Server) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func q(v string) string { return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(v) + `"` }

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	var user, pass string
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		f := strings.Fields(strings.TrimSpace(line))
		if len(f) == 0 {
			continue
		}
		s.mu.Lock()
		delay, stale, broken := s.delay, s.stale, s.broken
		needUser, needPass := s.user, s.pass
		ups := append([]UPS(nil), s.ups...)
		s.mu.Unlock()
		time.Sleep(delay)
		w := func(l string) { fmt.Fprint(conn, l+"\n") }
		switch {
		case f[0] == "USERNAME" && len(f) == 2:
			user = strings.Trim(f[1], `"`)
			w("OK")
		case f[0] == "PASSWORD" && len(f) == 2:
			pass = strings.Trim(f[1], `"`)
			w("OK")
		case f[0] == "LOGOUT":
			w("OK Goodbye")
			return
		case len(f) == 2 && f[0] == "LIST" && f[1] == "UPS":
			w("BEGIN LIST UPS")
			for _, u := range ups {
				w("UPS " + u.Name + " " + q(u.Desc))
			}
			w("END LIST UPS")
		case len(f) == 3 && f[0] == "LIST" && f[1] == "VAR":
			if needUser != "" && (user != needUser || pass != needPass) {
				w("ERR ACCESS-DENIED")
				continue
			}
			var u *UPS
			for i := range ups {
				if ups[i].Name == f[2] {
					u = &ups[i]
				}
			}
			switch {
			case u == nil:
				w("ERR UNKNOWN-UPS")
			case stale:
				w("ERR DATA-STALE")
			case broken:
				w("BEGIN LIST VAR " + u.Name)
				w(`VAR ` + u.Name + ` ups.status "OL`)
				w("END LIST VAR " + u.Name)
			default:
				w("BEGIN LIST VAR " + u.Name)
				s.mu.Lock()
				keys := make([]string, 0, len(u.Vars))
				for k := range u.Vars {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					w("VAR " + u.Name + " " + k + " " + q(u.Vars[k]))
				}
				s.mu.Unlock()
				w("END LIST VAR " + u.Name)
			}
		default:
			w("ERR UNKNOWN-COMMAND")
		}
	}
}
