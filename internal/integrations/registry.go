package integrations

import (
	"fmt"
	"sort"
)

// Registry kennt alle verfügbaren Integrationstypen.
type Registry struct {
	byType map[string]Integration
}

func NewRegistry(list ...Integration) *Registry {
	r := &Registry{byType: map[string]Integration{}}
	for _, i := range list {
		r.Register(i)
	}
	return r
}

// Register fügt einen Typ hinzu; doppelte Typen sind ein Programmierfehler.
func (r *Registry) Register(i Integration) {
	if _, dup := r.byType[i.Type()]; dup {
		panic(fmt.Sprintf("integration type %q registered twice", i.Type()))
	}
	for _, f := range i.Schema().Fields {
		if f.Type == FieldSecret && f.Default != nil {
			panic(fmt.Sprintf("integration %q: secret field %q must not have a default", i.Type(), f.Key))
		}
		for _, sub := range f.Item {
			if sub.Type == FieldSecret || sub.Type == FieldList {
				panic(fmt.Sprintf("integration %q: list %q must not contain %s fields", i.Type(), f.Key, sub.Type))
			}
		}
	}
	r.byType[i.Type()] = i
}

func (r *Registry) Get(typ string) (Integration, bool) {
	i, ok := r.byType[typ]
	return i, ok
}

// All liefert alle Typen, sortiert nach Bezeichner.
func (r *Registry) All() []Integration {
	out := make([]Integration, 0, len(r.byType))
	for _, i := range r.byType {
		out = append(out, i)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Type() < out[b].Type() })
	return out
}
