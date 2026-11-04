package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newLog(t *testing.T) (*Log, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "audit.log")
	l, err := Open(path, "rome")
	if err != nil {
		t.Fatal(err)
	}
	tick := time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)
	l.clock = func() time.Time { tick = tick.Add(time.Second); return tick }
	return l, path
}

func TestEntriesAreRecordedNewestFirstWithActor(t *testing.T) {
	l, path := newLog(t)
	l.Add("connect", "Atlas Server", "ok")
	l.Add("container.exec", "Atlas Server", "container=app user=root sudo=true")
	l.Add("disconnect", "Atlas Server", "")
	got, err := l.Read(10)
	if err != nil || len(got) != 3 {
		t.Fatalf("%v %v", got, err)
	}
	if got[0].Event != "disconnect" || got[2].Event != "connect" || got[0].Actor != "rome" || got[1].Subject != "Atlas Server" {
		t.Fatalf("%+v", got)
	}
	if got, _ := l.Read(2); len(got) != 2 || got[0].Event != "disconnect" {
		t.Fatalf("limit: %+v", got)
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", st.Mode().Perm())
	}
}

func TestChainDetectsEditedDeletedAndReorderedLines(t *testing.T) {
	l, path := newLog(t)
	for _, e := range []string{"a", "b", "c", "d"} {
		l.Add(e, "s", "")
	}
	if r := Verify(path); !r.OK || r.Entries != 4 {
		t.Fatalf("untouched log: %+v", r)
	}
	orig, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimSpace(string(orig)), "\n")
	cases := map[string][]string{
		"edited":    {lines[0], strings.Replace(lines[1], `"event":"b"`, `"event":"x"`, 1), lines[2], lines[3]},
		"deleted":   {lines[0], lines[2], lines[3]},
		"reordered": {lines[0], lines[2], lines[1], lines[3]},
	}
	for name, ls := range cases {
		os.WriteFile(path, []byte(strings.Join(ls, "\n")+"\n"), 0o600)
		if r := Verify(path); r.OK || r.BadLine == 0 {
			t.Errorf("%s went unnoticed: %+v", name, r)
		}
	}
}

func TestTheChainContinuesAfterReopening(t *testing.T) {
	l, path := newLog(t)
	l.Add("one", "s", "")
	l2, err := Open(path, "rome")
	if err != nil {
		t.Fatal(err)
	}
	l2.Add("two", "s", "")
	if r := Verify(path); !r.OK || r.Entries != 2 {
		t.Fatalf("%+v", r)
	}
}

func TestDetailIsCleanedAndNeverMultiLine(t *testing.T) {
	l, path := newLog(t)
	l.Add("connect.fail", "S", "line one\nline two\r\n"+strings.Repeat("x", 1000))
	raw, _ := os.ReadFile(path)
	if strings.Count(string(raw), "\n") != 1 {
		t.Fatal("an entry must stay on one line")
	}
	got, _ := l.Read(1)
	if len(got[0].Detail) > 300 {
		t.Errorf("detail not truncated (%d)", len(got[0].Detail))
	}
	if r := Verify(path); !r.OK {
		t.Fatalf("%+v", r)
	}
}

func TestRotationKeepsTheNewestFilesAndOpensAFreshChain(t *testing.T) {
	l, path := newLog(t)
	l.maxBytes, l.keep = 300, 2
	for i := 0; i < 40; i++ {
		l.Add("event", "server", "some detail to make the line longer")
	}
	for _, p := range []string{path, path + ".1", path + ".2"} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s: %v", p, err)
		}
		if r := Verify(p); !r.OK {
			t.Errorf("%s does not verify: %+v", p, r)
		}
	}
	if _, err := os.Stat(path + ".3"); err == nil {
		t.Error("kept more files than asked")
	}
}

func TestVerifyOfAMissingFileIsAnEmptyChain(t *testing.T) {
	if r := Verify(filepath.Join(t.TempDir(), "none")); !r.OK || r.Entries != 0 {
		t.Fatalf("%+v", r)
	}
}
