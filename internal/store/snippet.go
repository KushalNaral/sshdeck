package store

import "strings"

// Snippet is a saved shell command, run in a terminal or on several servers at once.
type Snippet struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Command string `json:"command"`
	Note    string `json:"note,omitempty"`
}

func (s *Store) Snippets() []Snippet {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Snippet{}, s.d.Snippets...)
}

// SaveSnippet adds a snippet, or replaces the one with the same id.
func (s *Store) SaveSnippet(n Snippet) (Snippet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locked {
		return n, ErrLocked
	}
	n.Name, n.Command = strings.TrimSpace(n.Name), strings.TrimSpace(n.Command)
	switch {
	case n.Name == "" || hasControl(n.Name):
		return n, invalid("snippet needs a name")
	case n.Command == "":
		return n, invalid("snippet needs a command")
	case len(n.Command) > 8192:
		return n, invalid("a snippet is at most 8 KB")
	}
	if n.ID == "" {
		n.ID = newID(n.Name)
		s.d.Snippets = append(s.d.Snippets, n)
	} else {
		found := false
		for i := range s.d.Snippets {
			if s.d.Snippets[i].ID == n.ID {
				s.d.Snippets[i], found = n, true
			}
		}
		if !found {
			s.d.Snippets = append(s.d.Snippets, n)
		}
	}
	return n, s.flush()
}

func (s *Store) DeleteSnippet(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locked {
		return ErrLocked
	}
	for i := range s.d.Snippets {
		if s.d.Snippets[i].ID == id {
			s.d.Snippets = append(s.d.Snippets[:i], s.d.Snippets[i+1:]...)
			return s.flush()
		}
	}
	return ErrNotFound
}
