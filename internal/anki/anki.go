// Package anki talks to the AnkiConnect add-on, the local HTTP API EasyDict
// uses to save a looked-up word without embedding Anki's database format.
//
// AnkiConnect listens on the loopback interface, so the client here ignores the
// app's proxy: routing 127.0.0.1 through a remote proxy would fail on every
// machine that configured one for the dictionary services.
package anki

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// DefaultEndpoint is the address AnkiConnect ships with.
const DefaultEndpoint = "http://127.0.0.1:8765"

// tag marks every note this app creates, so a shared deck can tell them apart.
const tag = "Transdict"

// Field is one note-model field together with the template that fills it. The
// template may use any of the Note variables; an empty one means {Word} on the
// first field and {FullResult} on the rest, which is what a Basic note wants.
type Field struct {
	Name     string
	Template string
}

// Note is the looked-up material a template draws on. Its fields mirror
// EasyDict's anki.easydict_field.* variables, so a note type mapped for
// EasyDict fills the same here.
type Note struct {
	Word           string `json:"word"`
	Translation    string `json:"translation"`
	Definition     string `json:"definition"`
	DictionaryText string `json:"dictionaryText"`
	DictionaryHTML string `json:"dictionaryHTML"`
	Etymology      string `json:"etymology"`
	Exchange       string `json:"exchange"`
	FullResult     string `json:"fullResult"`
	Phonetic       string `json:"phonetic"`
	Related        string `json:"related"`
}

// Client is one configured destination: which AnkiConnect, which deck, which
// note model, and the fields that model receives.
type Client struct {
	Endpoint string
	Deck     string
	Model    string
	Fields   []Field
}

// local is a plain client with no proxy and a short wait: AnkiConnect either
// answers from this machine or is not running.
var local = &http.Client{
	Timeout:   5 * time.Second,
	Transport: &http.Transport{Proxy: nil},
}

type request struct {
	Action  string `json:"action"`
	Version int    `json:"version"`
	Params  any    `json:"params,omitempty"`
}

type response struct {
	Result json.RawMessage `json:"result"`
	Error  *string         `json:"error"`
}

// Add creates one note for the looked-up word. The deck is created first, so a
// fresh Anki profile with no deck of that name still accepts the first card.
func (c Client) Add(note Note) error {
	note.Word = strings.TrimSpace(note.Word)
	if note.Word == "" {
		return errors.New("没有可添加到 Anki 的单词")
	}
	fields := c.fields(note)
	if deck := c.deck(); deck != "" {
		if _, err := c.call("createDeck", map[string]any{"deck": deck}); err != nil {
			return err
		}
	}
	payload := map[string]any{
		"deckName":  c.deck(),
		"modelName": c.model(),
		"fields":    fields,
		"options":   map[string]any{"allowDuplicate": false},
		"tags":      []string{tag},
	}
	_, err := c.call("addNote", map[string]any{"note": payload})
	return err
}

// Test reports the AnkiConnect version and checks that the configured deck and
// model exist, the two mistakes a valid endpoint can still have.
func (c Client) Test() (int, error) {
	raw, err := c.call("version", nil)
	if err != nil {
		return 0, err
	}
	var version int
	if err := json.Unmarshal(raw, &version); err != nil {
		return 0, errors.New("AnkiConnect 响应无法解析")
	}
	if err := c.has("deckNames", "deck", c.deck()); err != nil {
		return 0, err
	}
	if err := c.has("modelNames", "model", c.model()); err != nil {
		return 0, err
	}
	return version, nil
}

// Fields lists the note model's field names. Anki spells them its own way —
// 单词, 释义, Front — and a field name typed by hand is the easiest way to get a
// note the user cannot see; the settings page offers this list instead.
func Fields(endpoint, model string) ([]string, error) {
	c := Client{Endpoint: endpoint, Model: model}
	raw, err := c.call("modelFieldNames", map[string]any{"modelName": c.model()})
	if err != nil {
		return nil, err
	}
	var names []string
	if err := json.Unmarshal(raw, &names); err != nil {
		return nil, errors.New("AnkiConnect 响应无法解析")
	}
	return names, nil
}

// has asks AnkiConnect to list one kind of name and reports whether want is in
// it. deckNames and modelNames take no arguments, so the two checks differ only
// in the action and the sentence a miss produces.
func (c Client) has(action, kind, want string) error {
	if want == "" {
		return nil
	}
	raw, err := c.call(action, nil)
	if err != nil {
		return err
	}
	var names []string
	if err := json.Unmarshal(raw, &names); err != nil {
		return errors.New("AnkiConnect 响应无法解析")
	}
	for _, name := range names {
		if name == want {
			return nil
		}
	}
	if kind == "deck" {
		return errors.New("Anki 中没有这个牌组")
	}
	return errors.New("Anki 中没有这个模型")
}

func (c Client) fields(note Note) map[string]string {
	list := c.Fields
	if len(list) == 0 {
		list = []Field{{Name: "Front", Template: "{Word}"}, {Name: "Back", Template: "{FullResult}"}}
	}
	out := make(map[string]string, len(list))
	for i, f := range list {
		name := strings.TrimSpace(f.Name)
		if name == "" {
			continue
		}
		tpl := f.Template
		if strings.TrimSpace(tpl) == "" {
			if i == 0 {
				tpl = "{Word}"
			} else {
				tpl = "{FullResult}"
			}
		}
		out[name] = expand(tpl, note)
	}
	// A page that saved only nameless rows still has to produce a note.
	if len(out) == 0 {
		out["Front"] = expand("{Word}", note)
	}
	return out
}

// tokenPattern matches one template variable. Unknown names are left alone so a
// typo such as "{Meaning}" stays visible instead of silently emptying a field.
var tokenPattern = regexp.MustCompile(`\{([A-Za-z]+)\}`)

// expand renders one template. Matching is case-insensitive, as in EasyDict,
// and every value but {DictionaryHTML} is escaped the way Anki expects: the
// field is HTML, so a meaning that contains "<" would otherwise disappear.
func expand(tpl string, note Note) string {
	values := map[string]string{
		"word":           note.Word,
		"translation":    note.Translation,
		"definition":     note.Definition,
		"dictionarytext": note.DictionaryText,
		"etymology":      note.Etymology,
		"exchange":       note.Exchange,
		"fullresult":     note.FullResult,
		"phonetic":       note.Phonetic,
		"related":        note.Related,
	}
	out := tokenPattern.ReplaceAllStringFunc(tpl, func(match string) string {
		name := match[1 : len(match)-1]
		if strings.EqualFold(name, "DictionaryHTML") {
			return note.DictionaryHTML
		}
		value, ok := values[strings.ToLower(name)]
		if !ok {
			return match
		}
		return escapeHTML(value)
	})
	return strings.TrimSpace(out)
}

// escapeHTML mirrors EasyDict's renderer: entities first, then newlines become
// <br> so a multi-line meaning keeps its shape in the note.
func escapeHTML(text string) string {
	escaped := strings.NewReplacer(
		"&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;",
	).Replace(text)
	escaped = strings.ReplaceAll(escaped, "\r\n", "\n")
	escaped = strings.ReplaceAll(escaped, "\r", "\n")
	return strings.ReplaceAll(escaped, "\n", "<br>")
}

func (c Client) endpoint() string {
	e := strings.TrimRight(strings.TrimSpace(c.Endpoint), "/")
	if e == "" {
		return DefaultEndpoint
	}
	return e
}

func (c Client) deck() string {
	if d := strings.TrimSpace(c.Deck); d != "" {
		return d
	}
	return "Default"
}

func (c Client) model() string {
	if m := strings.TrimSpace(c.Model); m != "" {
		return m
	}
	return "Basic"
}

func (c Client) call(action string, params any) (json.RawMessage, error) {
	body, err := json.Marshal(request{Action: action, Version: 6, Params: params})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, c.endpoint(), bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("Anki Connect 地址无效")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := local.Do(req)
	if err != nil {
		log.Printf("anki: %v", err)
		return nil, errors.New("无法连接 AnkiConnect")
	}
	defer resp.Body.Close()
	var out response
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, errors.New("AnkiConnect 响应无法解析")
	}
	if out.Error != nil && *out.Error != "" {
		return nil, fmt.Errorf("AnkiConnect：%s", *out.Error)
	}
	return out.Result, nil
}
