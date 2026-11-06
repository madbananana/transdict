package capture

import (
	"crypto/rand"
	"encoding/hex"
	"sort"
	"sync"
	"time"
)

// Store keeps frozen frames between the shortcut press and the selection, so a
// session id is all the overlay needs to reference its own display image.
type Store struct {
	mu       sync.Mutex
	ids      []string
	frames   map[string][]Frame
	previews map[string]map[int][]byte
	crops    map[string][]byte
	created  map[string]time.Time
}

func NewStore() *Store {
	return &Store{
		frames:   map[string][]Frame{},
		previews: map[string]map[int][]byte{},
		crops:    map[string][]byte{},
		created:  map[string]time.Time{},
	}
}

const keepSessions = 3

func (s *Store) NewSession(frames []Frame) string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	id := hex.EncodeToString(buf)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.frames[id] = frames
	s.created[id] = time.Now()
	s.ids = append(s.ids, id)
	for len(s.ids) > keepSessions {
		old := s.ids[0]
		s.ids = s.ids[1:]
		delete(s.frames, old)
		delete(s.previews, old)
		delete(s.crops, old)
		delete(s.created, old)
	}
	return id
}

func (s *Store) Frame(id string, index int) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range s.frames[id] {
		if f.Index == index {
			return f.PNG, true
		}
	}
	return nil, false
}

// Preview is the display copy of a frame: the same image as a much smaller
// JPEG, encoded once per session and frame. A frame that cannot be re-encoded
// is served as the original PNG, which only costs the extra transfer.
func (s *Store) Preview(id string, index int) ([]byte, bool) {
	s.mu.Lock()
	if p := s.previews[id]; p != nil {
		if data, ok := p[index]; ok {
			s.mu.Unlock()
			return data, true
		}
	}
	var png []byte
	for _, f := range s.frames[id] {
		if f.Index == index {
			png = f.PNG
			break
		}
	}
	s.mu.Unlock()
	if png == nil {
		return nil, false
	}

	data, err := ForDisplay(png)
	if err != nil {
		return png, true
	}
	s.mu.Lock()
	if s.previews[id] == nil {
		s.previews[id] = map[int][]byte{}
	}
	s.previews[id][index] = data
	s.mu.Unlock()
	return data, true
}

func (s *Store) PutCrop(id string, crop []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.crops[id] = crop
}

func (s *Store) Crop(id string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.crops[id]
	return data, ok
}

func (s *Store) Session(id string) ([]Frame, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	frames, ok := s.frames[id]
	if !ok {
		return nil, false
	}
	out := append([]Frame(nil), frames...)
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out, true
}
