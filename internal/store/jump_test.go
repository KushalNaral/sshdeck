package store

import (
	"strings"
	"testing"
)

func saveAs(t *testing.T, s *Store, name, jump string) Server {
	t.Helper()
	sv, err := s.SaveServer(Server{Name: name, Host: name, Port: 22, User: "u", Auth: "password", JumpID: jump}, "pw")
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return sv
}

func TestJumpHostRules(t *testing.T) {
	s, _, _ := open(t)
	a := saveAs(t, s, "a", "")
	b := saveAs(t, s, "b", a.ID)
	c := saveAs(t, s, "c", b.ID)
	d := saveAs(t, s, "d", c.ID) // three hops: c, b, a

	e := Server{Name: "e", Host: "e", Port: 22, User: "u", Auth: "password", JumpID: d.ID}
	if _, err := s.SaveServer(e, "pw"); err == nil {
		t.Error("a fourth hop should be refused")
	}
	if _, err := s.SaveServer(Server{Name: "x", Host: "x", Port: 22, User: "u", Auth: "password", JumpID: "nope"}, "pw"); err == nil {
		t.Error("an unknown jump host should be refused")
	}
	a.JumpID = a.ID
	if _, err := s.SaveServer(a, ""); err == nil {
		t.Error("a server cannot jump through itself")
	}
	a.JumpID = d.ID // a -> d -> c -> b -> a
	if _, err := s.SaveServer(a, ""); err == nil {
		t.Error("a loop should be refused")
	}
}

func TestJumpHostCannotBeDeletedWhileUsed(t *testing.T) {
	s, _, _ := open(t)
	a := saveAs(t, s, "bastion", "")
	b := saveAs(t, s, "db", a.ID)
	err := s.DeleteServer(a.ID)
	if err == nil || !strings.Contains(err.Error(), "db") {
		t.Fatalf("should name the servers that need it: %v", err)
	}
	if err := s.DeleteServer(b.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteServer(a.ID); err != nil {
		t.Fatal(err)
	}
}
