package gui

import (
	"transdict/internal/dict"
	"transdict/internal/history"
	"transdict/internal/l10n"
	"transdict/internal/langs"
	"transdict/internal/ocr"
	"transdict/internal/query"
	"transdict/internal/settings"
	"transdict/internal/translate"
	"transdict/internal/tts"
)

// The settings tables are drawn by the webview but worded by Go, so this is
// where the interface language reaches them. Each helper copies rather than
// editing in place: the label and language tables are package-level data shared
// by every window, and overwriting them would rewrite the Chinese source too.
func localizedLabels(in []settings.Label) []settings.Label {
	out := make([]settings.Label, len(in))
	for i, l := range in {
		out[i] = settings.Label{Key: l.Key, Name: l10n.T(l.Name)}
	}
	return out
}

func localizedLangs(in []langs.Lang) []langs.Lang {
	out := make([]langs.Lang, len(in))
	for i, l := range in {
		l.Name = l10n.T(l.Name)
		out[i] = l
	}
	return out
}

// localizedProviders words the service rows for the window that will draw them.
// A provider keeps the name it was created with in the store, which is Chinese,
// so an untouched row follows the interface language here; the table only holds
// those default names, so a row the user invented falls through unchanged.
func localizedProviders(cfg settings.Config) settings.Config {
	for _, category := range []string{settings.CatOCR, settings.CatTranslate, settings.CatDict, settings.CatTTS} {
		out := append([]settings.Provider(nil), cfg.List(category)...)
		for i, p := range out {
			p.Label = l10n.T(p.Label)
			out[i] = p
		}
		if len(out) > 0 {
			setList(&cfg, category, out)
		}
	}
	return cfg
}

// localizedResponse words a result for the window that will draw it. Service
// labels are the names stored in the config — Chinese defaults — so an untouched
// service reads in the interface language and one the user renamed falls through
// the table unchanged. The copies matter: the same response is also being
// recorded in history, which keeps the stored wording.
//
// The per-service error lines are worded by the providers, so they are Go's text
// too and travel with the label. A panel that showed one untranslated would put
// Chinese in the middle of an English result.
func localizedResponse(res query.Response) query.Response {
	out := res
	if res.OCR != nil {
		out.OCR = append([]ocr.Outcome(nil), res.OCR...)
		for i, o := range out.OCR {
			o.Label = l10n.T(o.Label)
			o.Error = l10n.T(o.Error)
			out.OCR[i] = o
		}
	}
	if res.Dict != nil {
		out.Dict = append([]dict.Result(nil), res.Dict...)
		for i, d := range out.Dict {
			d.Label = l10n.T(d.Label)
			d.Error = l10n.T(d.Error)
			out.Dict[i] = d
		}
	}
	if res.Translate != nil {
		out.Translate = append([]translate.Result(nil), res.Translate...)
		for i, tr := range out.Translate {
			tr.Label = l10n.T(tr.Label)
			tr.Error = l10n.T(tr.Error)
			out.Translate[i] = tr
		}
	}
	out.Error = l10n.T(res.Error)
	if res.Skipped != nil {
		out.Skipped = append([]string(nil), res.Skipped...)
		for i, name := range out.Skipped {
			out.Skipped[i] = l10n.T(name)
		}
	}
	return out
}

// localizedSpeech words a read-aloud attempt. The webview plays the clip or
// reads the error back to the user, and only the clip itself is language-free.
func localizedSpeech(r tts.Result) tts.Result {
	r.Label = l10n.T(r.Label)
	r.Error = l10n.T(r.Error)
	return r
}

// localizedHistory does the same for the stored rows of 历史: the entry keeps the
// wording it was recorded with, and only the copy handed to a window is relabelled.
func localizedHistory(in []history.Entry) []history.Entry {
	out := append([]history.Entry(nil), in...)
	for i, e := range out {
		if e.Services == nil {
			continue
		}
		svcs := append([]string(nil), e.Services...)
		for j, name := range svcs {
			svcs[j] = l10n.T(name)
		}
		out[i].Services = svcs
	}
	return out
}

func localizedTypes(in []settings.ServiceType) []settings.ServiceType {
	out := make([]settings.ServiceType, len(in))
	for i, t := range in {
		t.Name = l10n.T(t.Name)
		t.Note = l10n.T(t.Note)
		if t.Fields != nil {
			t.Fields = append([]settings.Field(nil), t.Fields...)
			for j, f := range t.Fields {
				f.Label = l10n.T(f.Label)
				f.Placeholder = l10n.T(f.Placeholder)
				t.Fields[j] = f
			}
		}
		out[i] = t
	}
	return out
}
