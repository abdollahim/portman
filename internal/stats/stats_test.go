package stats

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

var zone = time.FixedZone("TEST", 3*3600+1800)

func at(day, hour, minute int) time.Time {
	return time.Date(2026, time.November, day, hour, minute, 0, 0, zone)
}

func openFake(t *testing.T, rx, tx *uint64) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "state.json"), "eth0", zone)
	if err != nil {
		t.Fatal(err)
	}
	s.read = func(string) (uint64, uint64, error) { return *rx, *tx, nil }
	return s
}

func TestDayRolloverAndYesterday(t *testing.T) {
	var rx, tx uint64
	s := openFake(t, &rx, &tx)
	s.Sample(at(23, 23, 0)) // baseline
	rx, tx = 100, 10
	s.Sample(at(23, 23, 59))
	rx, tx = 300, 40
	s.Sample(at(24, 0, 1))

	snap := s.Snapshot(at(24, 0, 2))
	if snap.Today.NICRx != 200 || snap.Today.NICTx != 30 {
		t.Fatalf("today = %+v, want rx 200 tx 30", snap.Today)
	}
	if snap.Yesterday.NICRx != 100 || snap.Yesterday.NICTx != 10 {
		t.Fatalf("yesterday = %+v, want rx 100 tx 10", snap.Yesterday)
	}
}

func TestRebootIsCountedFromZero(t *testing.T) {
	var rx, tx uint64
	s := openFake(t, &rx, &tx)
	rx, tx = 1000, 2000
	s.Sample(at(24, 10, 0))
	rx, tx = 1500, 2600
	s.Sample(at(24, 10, 1))
	rx, tx = 80, 20 // counters restarted after a reboot
	s.Sample(at(24, 10, 2))

	snap := s.Snapshot(at(24, 10, 3))
	if snap.Today.NICRx != 580 || snap.Today.NICTx != 620 {
		t.Fatalf("today = %+v, want rx 580 tx 620", snap.Today)
	}
}

func TestMonthTotalsAndBillable(t *testing.T) {
	var rx, tx uint64
	s := openFake(t, &rx, &tx)
	s.Sample(at(1, 12, 0))
	rx, tx = 1000, 100
	s.Sample(at(1, 12, 1))
	rx, tx = 3000, 400
	s.Sample(at(2, 12, 0))

	m := s.Snapshot(at(2, 12, 1)).Month
	if m.NICRx != 3000 || m.NICTx != 400 {
		t.Fatalf("month = %+v", m)
	}
	if got := m.Billable("sum"); got != 3400 {
		t.Fatalf("sum = %d, want 3400", got)
	}
	if got := m.Billable("max"); got != 3000 {
		t.Fatalf("max = %d, want 3000", got)
	}
}

func TestBridgeCountersSurviveSaveAndReopen(t *testing.T) {
	var rx, tx uint64
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := Open(path, "eth0", zone)
	if err != nil {
		t.Fatal(err)
	}
	s.read = func(string) (uint64, uint64, error) { return rx, tx, nil }

	s.AddConn()
	s.AddUp(5)
	s.AddDown(7)
	s.Sample(time.Now())
	rx, tx = 500, 50
	s.Sample(time.Now())
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	again, err := Open(path, "eth0", zone)
	if err != nil {
		t.Fatal(err)
	}
	snap := again.Snapshot(time.Now())
	if snap.Today.BridgeConns != 1 || snap.Today.BridgeUp != 5 || snap.Today.BridgeDown != 7 {
		t.Fatalf("bridge totals lost: %+v", snap.Today)
	}
	if snap.Today.NICRx != 500 || snap.Today.NICTx != 50 {
		t.Fatalf("interface totals lost: %+v", snap.Today)
	}
}

func TestCorruptStateIsMovedAside(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path, "eth0", zone)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".corrupt"); err != nil {
		t.Fatal("corrupt file was not moved aside")
	}
	if s.Snapshot(time.Now()).Today != (Day{}) {
		t.Fatal("expected empty state after corruption")
	}
}

func TestParseNetDev(t *testing.T) {
	data := []byte(`Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo:    1000      10    0    0    0     0          0         0     1000      10    0    0    0     0       0          0
  eth0: 123456     100    0    0    0     0          0         0   654321      90    0    0    0     0       0          0
`)
	rx, tx, err := parseNetDev(data, "eth0")
	if err != nil {
		t.Fatal(err)
	}
	if rx != 123456 || tx != 654321 {
		t.Fatalf("rx=%d tx=%d, want 123456 and 654321", rx, tx)
	}
	if _, _, err := parseNetDev(data, "wlan9"); err == nil {
		t.Fatal("expected an error for a missing interface")
	}
}

func on(m time.Month, day, hour, minute int) time.Time {
	return time.Date(2026, m, day, hour, minute, 0, 0, zone)
}

func TestCycleStart(t *testing.T) {
	cases := []struct {
		now  time.Time
		day  int
		want string
	}{
		{on(time.November, 5, 12, 0), 27, "2026-10-27"},
		{on(time.November, 27, 0, 1), 27, "2026-11-27"},
		{on(time.November, 30, 12, 0), 27, "2026-11-27"},
		{on(time.November, 1, 12, 0), 1, "2026-11-01"},
	}
	for _, c := range cases {
		if got := CycleStart(c.now, c.day, zone); got != c.want {
			t.Fatalf("CycleStart(%v, %d) = %q, want %q", c.now, c.day, got, c.want)
		}
	}
}

func TestCycleExcludesPreviousCycle(t *testing.T) {
	var rx, tx uint64
	s := openFake(t, &rx, &tx)
	s.SetBillingDay(27)

	s.Sample(on(time.October, 26, 10, 0))
	rx = 100
	s.Sample(on(time.October, 26, 10, 1))
	rx = 300
	s.Sample(on(time.October, 28, 10, 0))
	rx = 600
	s.Sample(on(time.November, 28, 10, 0))

	snap := s.Snapshot(on(time.November, 28, 11, 0))
	if snap.CycleStart != "2026-11-27" {
		t.Fatalf("cycle start = %q, want 2026-11-27", snap.CycleStart)
	}
	if snap.Month.NICRx != 300 {
		t.Fatalf("cycle total = %d, want 300 (only 28 Nov)", snap.Month.NICRx)
	}
}

func TestCycleStartClampsToShortMonths(t *testing.T) {
	cases := []struct {
		now  time.Time
		day  int
		want string
	}{
		{on(time.February, 28, 9, 0), 31, "2026-02-28"}, // February has 28 days
		{on(time.February, 27, 9, 0), 31, "2026-01-31"}, // before February's reset
		{on(time.March, 1, 9, 0), 31, "2026-02-28"},
		{on(time.March, 31, 9, 0), 31, "2026-03-31"},
		{on(time.April, 29, 9, 0), 30, "2026-03-30"},
		{on(time.April, 30, 9, 0), 30, "2026-04-30"},
	}
	for _, c := range cases {
		if got := CycleStart(c.now, c.day, zone); got != c.want {
			t.Fatalf("CycleStart(%v, %d) = %q, want %q", c.now, c.day, got, c.want)
		}
	}
}

func TestBillingDayIsPerStore(t *testing.T) {
	var rx, tx uint64
	a := openFake(t, &rx, &tx)
	b := openFake(t, &rx, &tx)
	a.SetBillingDay(27)
	b.SetBillingDay(1)

	now := on(time.November, 26, 12, 0)
	if got := a.Snapshot(now).CycleStart; got != "2026-10-27" {
		t.Fatalf("store a cycle = %q, want 2026-10-27", got)
	}
	if got := b.Snapshot(now).CycleStart; got != "2026-11-01" {
		t.Fatalf("store b cycle = %q, want 2026-11-01", got)
	}
}