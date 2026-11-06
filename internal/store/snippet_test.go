package store

import "testing"

func TestSnippets(t *testing.T) {
	s, _, path := open(t)
	if _, err := s.SaveSnippet(Snippet{Name: "", Command: "ls"}); err == nil {
		t.Error("a snippet needs a name")
	}
	if _, err := s.SaveSnippet(Snippet{Name: "x", Command: "  "}); err == nil {
		t.Error("a snippet needs a command")
	}
	a, err := s.SaveSnippet(Snippet{Name: "Disk", Command: "df -h\n"})
	if err != nil || a.ID == "" || a.Command != "df -h" {
		t.Fatalf("%+v %v", a, err)
	}
	a.Command = "df -h /var"
	if b, err := s.SaveSnippet(a); err != nil || b.ID != a.ID || len(s.Snippets()) != 1 {
		t.Fatalf("saving with an id replaces: %+v %v", s.Snippets(), err)
	}
	s2, err := Open(path, memSecrets{})
	if err != nil || len(s2.Snippets()) != 1 || s2.Snippets()[0].Command != "df -h /var" {
		t.Fatalf("they must survive a restart: %+v %v", s2.Snippets(), err)
	}
	if err := s.DeleteSnippet(a.ID); err != nil || len(s.Snippets()) != 0 {
		t.Fatal(err)
	}
	if s.DeleteSnippet(a.ID) != ErrNotFound {
		t.Error("deleting twice should say not found")
	}
}
