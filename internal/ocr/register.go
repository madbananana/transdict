package ocr

import "transdict/internal/settings"

// Factory builds a provider from its persisted configuration. A nil provider
// with a nil error is not a valid return: say what is missing instead.
type Factory func(p settings.Provider) (Provider, error)

type entry struct {
	meta settings.ServiceType
	make Factory
}

// registry is filled by each provider file's init. Slice plus map: the slice
// keeps declaration order for the settings UI, the map serves lookups.
var registry = map[settings.ProviderKind]entry{}

// Register adds a service. Category is forced so a provider file cannot
// announce itself into the wrong pipeline stage.
func Register(meta settings.ServiceType, f Factory) {
	meta.Category = settings.CatOCR
	registry[meta.Kind] = entry{meta: meta, make: f}
}

func Types() []settings.ServiceType {
	out := make([]settings.ServiceType, 0, len(registry))
	for _, e := range registry {
		out = append(out, e.meta)
	}
	sortTypes(out)
	return out
}

func Meta(kind settings.ProviderKind) (settings.ServiceType, bool) {
	e, ok := registry[kind]
	if !ok {
		return settings.ServiceType{}, false
	}
	return e.meta, true
}

// New constructs the provider a configured entry refers to.
func New(p settings.Provider) (Provider, error) {
	e, ok := registry[p.Kind]
	if !ok {
		return nil, &UnknownKind{Kind: p.Kind}
	}
	return e.make(p)
}

type UnknownKind struct{ Kind string }

func (e *UnknownKind) Error() string { return "未知的 OCR 服务：" + e.Kind }

func sortTypes(t []settings.ServiceType) {
	for i := 1; i < len(t); i++ {
		for j := i; j > 0 && t[j].Kind < t[j-1].Kind; j-- {
			t[j], t[j-1] = t[j-1], t[j]
		}
	}
}

// Ready reports whether the entry has what it takes to be called.
func Ready(p settings.Provider) bool {
	m, ok := Meta(p.Kind)
	return ok && p.Enabled && p.IsReady(m)
}
