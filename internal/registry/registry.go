package registry

import "sync"

var (
	mu        sync.RWMutex
	resources []Resource
	byName    = make(map[string]Resource)
)

func Register(r Resource) {
	mu.Lock()
	defer mu.Unlock()
	if _, exists := byName[r.Name()]; exists {
		return
	}
	resources = append(resources, r)
	byName[r.Name()] = r
}

func All() []Resource {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Resource, len(resources))
	copy(out, resources)
	return out
}

func Get(name string) (Resource, bool) {
	mu.RLock()
	defer mu.RUnlock()
	r, ok := byName[name]
	return r, ok
}

func Reset() {
	mu.Lock()
	defer mu.Unlock()
	resources = nil
	byName = make(map[string]Resource)
}
