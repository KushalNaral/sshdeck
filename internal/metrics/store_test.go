package metrics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func smp(t int64, cpu float64) Sample {
	return Sample{T: t, HasRates: true, CPU: cpu, MemPct: 50, Disks: []Disk{{"/", 40}}}
}

func lines(t *testing.T, path string) int {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return len(strings.Split(strings.TrimSpace(string(b)), "\n"))
}

func TestStoreKeepsTheLatestAndTheLastHourInMemory(t *testing.T) {
	s := New(t.TempDir())
	for i := 0; i < 800; i++ { // more than the ring holds
		s.Add("web", smp(int64(1000+i*5), float64(i%100)))
	}
	last, ok := s.Latest("web")
	if !ok || last.T != 1000+799*5 {
		t.Fatalf("%+v %v", last, ok)
	}
	got := s.Range("web", time.Unix(last.T-3600, 0), time.Unix(last.T, 0), 10000)
	if len(got) > 720 || got[len(got)-1].T != last.T {
		t.Errorf("the hour is bounded and ends at the latest: %d points", len(got))
	}
	all := s.Range("web", time.Unix(0, 0), time.Unix(1<<40, 0), 10000)
	if all[len(all)-1].T != last.T {
		t.Error("a long range must still end at the newest sample")
	}
	if _, ok := s.Latest("nobody"); ok {
		t.Error("an unknown server has no latest")
	}
}

func TestMinutesAreAveragedToDiskWhenTheyEnd(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	// three minutes of 5 s samples starting on a minute boundary: 12 per minute
	for i := 0; i < 36; i++ {
		s.Add("web", smp(int64(6000+i*5), 10))
	}
	f := filepath.Join(dir, "web.jsonl")
	if n := lines(t, f); n != 2 { // the third minute is still open
		t.Fatalf("closed minutes on disk = %d, want 2", n)
	}
	s.Flush()
	if n := lines(t, f); n != 3 {
		t.Fatalf("after Flush = %d, want 3", n)
	}
	day := s.Range("web", time.Unix(0, 0), time.Unix(1<<40, 0), 600)
	if len(day) == 0 || day[0].CPU != 10 {
		t.Errorf("minute average: %+v", day)
	}
}

func TestLongRangesReadTheMinuteFileAndAreDownsampled(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	start := int64(1_700_000_040) // a minute boundary
	for m := 0; m < 3000; m++ {
		s.Add("web", smp(start+int64(m)*60, float64(m%100)))
		s.Add("web", smp(start+int64(m)*60+30, float64(m%100)))
	}
	s.Flush()
	from, to := time.Unix(start, 0), time.Unix(start+3000*60, 0)
	got := s.Range("web", from, to, 600)
	if len(got) == 0 || len(got) > 600 {
		t.Fatalf("points = %d, want 1..600", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i].T <= got[i-1].T {
			t.Fatal("points must be in time order")
		}
	}
}

func TestAfterARestartTheLastHourComesFromTheFile(t *testing.T) {
	dir := t.TempDir()
	a := New(dir)
	now := time.Now().Unix() / 60 * 60
	for m := 40; m >= 1; m-- {
		a.Add("web", smp(now-int64(m)*60, 20))
	}
	a.Flush()
	b := New(dir) // the app was restarted: the ring is empty
	got := b.Range("web", time.Unix(now-3600, 0), time.Unix(now, 0), 600)
	if len(got) < 30 {
		t.Errorf("history should survive a restart, got %d points", len(got))
	}
}

func TestPruneDropsOldMinutesOnly(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	now := time.Unix(1_700_000_000, 0)
	for _, age := range []time.Duration{9 * 24 * time.Hour, 8 * 24 * time.Hour, 6 * 24 * time.Hour, time.Hour} {
		s.Add("web", smp(now.Add(-age).Unix()/60*60, 5))
		s.Add("web", smp(now.Add(-age).Unix()/60*60+60, 5)) // closes the minute above
	}
	s.Flush()
	before := lines(t, filepath.Join(dir, "web.jsonl"))
	s.Prune(now, 7*24*time.Hour)
	after := lines(t, filepath.Join(dir, "web.jsonl"))
	if after >= before || after < 4 {
		t.Errorf("lines %d -> %d: the 8 and 9 day old minutes should go and the rest stay", before, after)
	}
}

func TestDeleteRemovesTheFileAndTheMemory(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	s.Add("web", smp(60, 1))
	s.Add("web", smp(120, 1))
	s.Flush()
	s.Delete("web")
	if _, err := os.Stat(filepath.Join(dir, "web.jsonl")); !os.IsNotExist(err) {
		t.Error("the file should be gone")
	}
	if _, ok := s.Latest("web"); ok {
		t.Error("memory should be cleared")
	}
}

func TestOddServerIdsNeverBecomePaths(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	for _, id := range []string{"../evil", "a/b", ".", ""} {
		s.Add(id, smp(60, 1))
		s.Add(id, smp(120, 1))
		s.Flush()
	}
	ents, _ := os.ReadDir(dir)
	if len(ents) != 0 {
		t.Errorf("nothing may be written for an unsafe id: %v", ents)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "evil.jsonl")); err == nil {
		t.Error("path traversal")
	}
}

func TestSamplesWithoutRatesDoNotPoisonAverages(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	s.Add("web", Sample{T: 6000, HasRates: false, CPU: 0})
	s.Add("web", smp(6005, 40))
	s.Add("web", smp(6010, 60))
	s.Add("web", smp(6060, 0)) // next minute closes the first
	got := s.Range("web", time.Unix(0, 0), time.Unix(1<<40, 0), 600)
	for _, p := range got {
		if p.T == 6000 && (p.CPU < 49 || p.CPU > 51) {
			t.Errorf("average of the two real readings is 50, got %.1f", p.CPU)
		}
	}
}
