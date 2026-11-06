package store

import "testing"

func tsrv(ts ...Tunnel) Server {
	return Server{Name: "web", Host: "h", Port: 22, User: "u", Auth: "password", Tunnels: ts}
}

func TestTunnelsAreChecked(t *testing.T) {
	ok := Tunnel{Name: "db", Kind: "local", Listen: "127.0.0.1:5433", Target: "db.internal:5432"}
	bad := map[string][]Tunnel{
		"kind":      {{Name: "x", Kind: "dynamic", Listen: "8080", Target: "a:1"}},
		"name":      {{Kind: "local", Listen: "8080", Target: "a:1"}},
		"port":      {{Name: "x", Kind: "local", Listen: "0", Target: "a:1"}},
		"target":    {{Name: "x", Kind: "local", Listen: "8080", Target: "nohost"}},
		"control":   {{Name: "x\n", Kind: "local", Listen: "8080", Target: "a:1"}},
		"duplicate": {ok, {Name: "again", Kind: "local", Listen: "127.0.0.1:5433", Target: "b:2"}},
	}
	for what, ts := range bad {
		if CheckServer(tsrv(ts...)) == nil {
			t.Errorf("%s: should be refused", what)
		}
	}
	if err := CheckServer(tsrv(ok)); err != nil {
		t.Fatal(err)
	}
}

func TestSavedTunnelsGetAnIDAndAListenAddress(t *testing.T) {
	s, _, _ := open(t)
	got, err := s.SaveServer(tsrv(Tunnel{Name: "web", Kind: "local", Listen: "8080", Target: "localhost:80"}), "pw")
	if err != nil {
		t.Fatal(err)
	}
	tn := got.Tunnels[0]
	if tn.ID == "" || tn.Listen != "127.0.0.1:8080" {
		t.Fatalf("%+v", tn)
	}
	again, err := s.SaveServer(got, "")
	if err != nil || again.Tunnels[0].ID != tn.ID {
		t.Fatalf("the id must stay put: %+v %v", again.Tunnels, err)
	}
}
