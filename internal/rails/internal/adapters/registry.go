package adapters

import "github.com/irfanali17899/fintech/internal/ledger/api"

type Registry struct{ rails map[string]api.Rail }

func NewRegistry(rails ...api.Rail) *Registry {
	r := &Registry{rails: make(map[string]api.Rail, len(rails))}
	for _, x := range rails {
		r.rails[x.Name()] = x
	}
	return r
}

func (r *Registry) Get(name string) (api.Rail, bool) {
	x, ok := r.rails[name]
	return x, ok
}
