package translate

import "transdict/internal/settings"

type Factory func(p settings.Provider) (Provider, error)

type entry struct {
	meta settings.ServiceType
	make Factory
}

var registry = map[settings.ProviderKind]entry{}

// Register adds a service. Category is forced so a provider file cannot
// announce itself into the wrong pipeline stage.
func Register(meta settings.ServiceType, f Factory) {
	meta.Category = settings.CatTranslate
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

// New constructs the provider a configured entry refers to. AI services and
// plain HTTP translators share this interface: an AI answer is a translation
// whose text happens to be Markdown.
func New(p settings.Provider) (Provider, error) {
	e, ok := registry[p.Kind]
	if !ok {
		return nil, &UnknownKind{Kind: p.Kind}
	}
	return e.make(p)
}

type UnknownKind struct{ Kind string }

func (e *UnknownKind) Error() string { return "未知的翻译服务：" + e.Kind }

func sortTypes(t []settings.ServiceType) {
	for i := 1; i < len(t); i++ {
		for j := i; j > 0 && t[j].Kind < t[j-1].Kind; j-- {
			t[j], t[j-1] = t[j-1], t[j]
		}
	}
}

func Ready(p settings.Provider) bool {
	m, ok := Meta(p.Kind)
	return ok && p.Enabled && p.IsReady(m)
}
