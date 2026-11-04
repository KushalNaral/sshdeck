package importer

import "testing"

func TestParseVersions(t *testing.T) {
	for in, want := range map[string][2]string{
		"U 22.04.5/ 8.3.19 / laravel":   {"Ubuntu 22.04.5", "8.3.19"},
		"CentOS : 7 / 7.4.27 / laravel": {"CentOS 7", "7.4.27"},
		"U 22.04 / x / laravel":         {"Ubuntu 22.04", ""},
		"":                              {"", ""},
	} {
		if os, php := ParseVersions(in); os != want[0] || php != want[1] {
			t.Errorf("%q = %q, %q; want %q, %q", in, os, php, want[0], want[1])
		}
	}
}

func TestPairsFirstWins(t *testing.T) {
	s := Sheet{Rows: [][]string{{"username", "a"}, {"IP:", "1.2.3.4"}, {"username:", "b"}}}
	m := s.Pairs(0)
	if m["username"] != "a" || m["ip"] != "1.2.3.4" {
		t.Fatalf("got %v", m)
	}
}
