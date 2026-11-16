package anki

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fake runs the AnkiConnect wire protocol over a local server, so the client is
// tested the way it is used rather than against a mock of its own structs.
type fake struct {
	*httptest.Server
	actions []string
	body    map[string]any
	results map[string]any
}

func newFake(t *testing.T, results map[string]any) *fake {
	t.Helper()
	f := &fake{results: results}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Action string          `json:"action"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("request is not JSON: %v", err)
			return
		}
		f.actions = append(f.actions, req.Action)
		if len(req.Params) > 0 {
			f.body = map[string]any{}
			_ = json.Unmarshal(req.Params, &f.body)
		}
		result, ok := f.results[req.Action]
		if !ok {
			result = nil
		}
		payload := map[string]any{"result": result, "error": nil}
		if err, ok := result.(string); ok && strings.HasPrefix(err, "err:") {
			payload = map[string]any{"result": nil, "error": strings.TrimPrefix(err, "err:")}
		}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	t.Cleanup(f.Close)
	return f
}

func TestAddCreatesDeckAndNote(t *testing.T) {
	f := newFake(t, map[string]any{"createDeck": 1, "addNote": 1730000000000.0})
	c := Client{
		Endpoint: f.URL, Deck: "英语", Model: "Basic",
		Fields: []Field{{Name: "Front", Template: "{Word}"}, {Name: "Back", Template: "{Translation}"}},
	}
	if err := c.Add(Note{Word: "hello", Translation: "喂，你好"}); err != nil {
		t.Fatal(err)
	}
	if len(f.actions) != 2 || f.actions[0] != "createDeck" || f.actions[1] != "addNote" {
		t.Fatalf("actions = %v, want createDeck then addNote", f.actions)
	}
	note := f.body["note"].(map[string]any)
	if note["deckName"] != "英语" || note["modelName"] != "Basic" {
		t.Errorf("note = %v", note)
	}
	fields := note["fields"].(map[string]any)
	if fields["Front"] != "hello" || fields["Back"] != "喂，你好" {
		t.Errorf("fields = %v", fields)
	}
	tags := note["tags"].([]any)
	if len(tags) != 1 || tags[0] != tag {
		t.Errorf("tags = %v", tags)
	}
	if dup := note["options"].(map[string]any)["allowDuplicate"]; dup != false {
		t.Errorf("allowDuplicate = %v, want false", dup)
	}
}

func TestAddExpandsConfiguredFields(t *testing.T) {
	f := newFake(t, map[string]any{"createDeck": 1, "addNote": 1.0})
	c := Client{
		Endpoint: f.URL, Deck: "D", Model: "Mine",
		Fields: []Field{{Name: "Word", Template: "{Word}（Transdict）"}, {Name: "Meaning", Template: "{Translation}"}},
	}
	if err := c.Add(Note{Word: "route", Translation: "路线"}); err != nil {
		t.Fatal(err)
	}
	fields := f.body["note"].(map[string]any)["fields"].(map[string]any)
	if fields["Word"] != "route（Transdict）" || fields["Meaning"] != "路线" {
		t.Errorf("fields = %v", fields)
	}
	if _, ok := fields["Front"]; ok {
		t.Errorf("fields = %v, the default names should be gone", fields)
	}
}

func TestAddFillsEveryMappedField(t *testing.T) {
	f := newFake(t, map[string]any{"createDeck": 1, "addNote": 1.0})
	c := Client{
		Endpoint: f.URL, Deck: "D", Model: "Mine",
		Fields: []Field{
			{Name: "单词", Template: "{Word}"},
			{Name: "释义", Template: "{Translation}"},
			{Name: "例句", Template: "{Word} — {Translation}"},
			{Name: "补充", Template: ""},
		},
	}
	if err := c.Add(Note{Word: "hello", Translation: "你好", FullResult: "你好"}); err != nil {
		t.Fatal(err)
	}
	fields := f.body["note"].(map[string]any)["fields"].(map[string]any)
	want := map[string]string{"单词": "hello", "释义": "你好", "例句": "hello — 你好", "补充": "你好"}
	for name, text := range want {
		if fields[name] != text {
			t.Errorf("字段 %s = %v，want %q", name, fields[name], text)
		}
	}
	if len(fields) != len(want) {
		t.Errorf("fields = %v", fields)
	}
}

func TestAddFillsEveryEasydictVariable(t *testing.T) {
	f := newFake(t, map[string]any{"createDeck": 1, "addNote": 1.0})
	c := Client{
		Endpoint: f.URL, Deck: "D", Model: "Mine",
		Fields: []Field{{Name: "All", Template: "{Word}|{Translation}|{Definition}|{Phonetic}|{Exchange}|{Related}|{FullResult}|{DictionaryText}|{DictionaryHTML}|{Etymology}"}},
	}
	note := Note{
		Word: "hello", Translation: "你好", Definition: "int. 喂；你好",
		Phonetic: "[həˈləʊ]", Exchange: "复数：hellos", Related: "Hello Kitty：凯蒂猫",
		FullResult: "hello\nint. 喂；你好", DictionaryText: "词典文本",
		DictionaryHTML: "<b>hello</b>", Etymology: "源自古英语",
	}
	if err := c.Add(note); err != nil {
		t.Fatal(err)
	}
	fields := f.body["note"].(map[string]any)["fields"].(map[string]any)
	want := "hello|你好|int. 喂；你好|[həˈləʊ]|复数：hellos|Hello Kitty：凯蒂猫|" +
		"hello<br>int. 喂；你好|词典文本|<b>hello</b>|源自古英语"
	if fields["All"] != want {
		t.Errorf("All = %q，want %q", fields["All"], want)
	}
}

func TestAddEscapesValuesAndKeepsUnknownTokens(t *testing.T) {
	f := newFake(t, map[string]any{"createDeck": 1, "addNote": 1.0})
	c := Client{
		Endpoint: f.URL, Deck: "D", Model: "Mine",
		Fields: []Field{{Name: "F", Template: "{word} <z> {Meaning}"}},
	}
	if err := c.Add(Note{Word: `a<b & "c"`, Translation: "x\ny"}); err != nil {
		t.Fatal(err)
	}
	fields := f.body["note"].(map[string]any)["fields"].(map[string]any)
	want := `a&lt;b &amp; &quot;c&quot; <z> {Meaning}`
	if fields["F"] != want {
		t.Errorf("F = %q，want %q", fields["F"], want)
	}
}

func TestAddUsesFullResultForUnnamedTemplates(t *testing.T) {
	f := newFake(t, map[string]any{"createDeck": 1, "addNote": 1.0})
	c := Client{Endpoint: f.URL, Deck: "D", Model: "Basic"}
	if err := c.Add(Note{Word: "hello", Translation: "你好", FullResult: "完整释义"}); err != nil {
		t.Fatal(err)
	}
	fields := f.body["note"].(map[string]any)["fields"].(map[string]any)
	if fields["Front"] != "hello" || fields["Back"] != "完整释义" {
		t.Errorf("fields = %v，空模板应是 {Word} 与 {FullResult}", fields)
	}
}

func TestAddRefusesAnEmptyWord(t *testing.T) {
	c := Client{}
	if err := c.Add(Note{Word: "  ", Translation: "x"}); err == nil {
		t.Fatal("empty word was accepted")
	}
}

func TestAddReportsAnkiConnectErrors(t *testing.T) {
	f := newFake(t, map[string]any{
		"createDeck": 1,
		"addNote":    "err:cannot create note because it is a duplicate",
	})
	c := Client{Endpoint: f.URL, Deck: "Default", Model: "Basic"}
	err := c.Add(Note{Word: "hello", Translation: "你好"})
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("err = %v, want the AnkiConnect sentence", err)
	}
}

func TestAddReportsAnOfflineEndpoint(t *testing.T) {
	c := Client{Endpoint: "http://127.0.0.1:1", Deck: "Default", Model: "Basic"}
	err := c.Add(Note{Word: "hello", Translation: "你好"})
	if err == nil || err.Error() != "无法连接 AnkiConnect" {
		t.Fatalf("err = %v, want 无法连接 AnkiConnect", err)
	}
}

func TestTestChecksVersionDeckAndModel(t *testing.T) {
	f := newFake(t, map[string]any{
		"version":    6,
		"deckNames":  []string{"Default", "英语"},
		"modelNames": []string{"Basic", "Cloze"},
	})
	c := Client{Endpoint: f.URL, Deck: "英语", Model: "Basic"}
	version, err := c.Test()
	if err != nil {
		t.Fatal(err)
	}
	if version != 6 {
		t.Errorf("version = %d, want 6", version)
	}
}

func TestTestNamesTheMissingPiece(t *testing.T) {
	f := newFake(t, map[string]any{
		"version":    6,
		"deckNames":  []string{"Default"},
		"modelNames": []string{"Basic"},
	})
	c := Client{Endpoint: f.URL, Deck: "英语", Model: "Basic"}
	if _, err := c.Test(); err == nil || err.Error() != "Anki 中没有这个牌组" {
		t.Fatalf("err = %v, want the missing deck", err)
	}
	c = Client{Endpoint: f.URL, Deck: "Default", Model: "Mine"}
	if _, err := c.Test(); err == nil || err.Error() != "Anki 中没有这个模型" {
		t.Fatalf("err = %v, want the missing model", err)
	}
}

func TestFieldsReadsTheModelsFieldNames(t *testing.T) {
	f := newFake(t, map[string]any{
		"modelFieldNames": []string{"单词", "释义"},
	})
	names, err := Fields(f.URL, "我的模型")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "单词" || names[1] != "释义" {
		t.Errorf("字段 = %v, want Anki 返回的两个名字", names)
	}
	if got := f.body["modelName"]; got != "我的模型" {
		t.Errorf("modelName = %v, want 配置里的模型", got)
	}
}

func TestFieldsReportsAnkiConnectErrors(t *testing.T) {
	f := newFake(t, map[string]any{
		"modelFieldNames": "err:model was not found",
	})
	if _, err := Fields(f.URL, "Missing"); err == nil || !strings.Contains(err.Error(), "model was not found") {
		t.Errorf("err = %v, want AnkiConnect 的错误", err)
	}
}
