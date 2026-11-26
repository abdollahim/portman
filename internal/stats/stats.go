// Package stats counts bridge traffic and network interface traffic per day,
// and keeps the totals in a state file so a restart doesn't lose them.
package stats

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"portman/internal/logging"
)

// keepDays is how much history is kept in the state file.
const keepDays = 65

// Recorder receives bridge traffic. *Store implements it.
type Recorder interface {
	AddConn()
	AddUp(n int64)
	AddDown(n int64)
}

// Discard ignores everything. It is the default when stats are off.
type Discard struct{}

func (Discard) AddConn()      {}
func (Discard) AddUp(int64)   {}
func (Discard) AddDown(int64) {}

// Day holds one day's totals. Bridge fields count tunnel traffic. NIC fields
// count everything the network interface sent and received.
type Day struct {
	BridgeConns int64 `json:"bridge_conns"`
	BridgeUp    int64 `json:"bridge_up"`
	BridgeDown  int64 `json:"bridge_down"`
	NICRx       int64 `json:"nic_rx"`
	NICTx       int64 `json:"nic_tx"`
}

func (d *Day) add(o Day) {
	d.BridgeConns += o.BridgeConns
	d.BridgeUp += o.BridgeUp
	d.BridgeDown += o.BridgeDown
	d.NICRx += o.NICRx
	d.NICTx += o.NICTx
}

// Billable returns the bytes counted against a monthly limit.
// "sum" adds received and sent bytes. "max" uses the larger of the two.
func (d Day) Billable(mode string) int64 {
	if mode == "max" {
		if d.NICRx > d.NICTx {
			return d.NICRx
		}
		return d.NICTx
	}
	return d.NICRx + d.NICTx
}

// Snapshot holds what a report needs: today, yesterday, and month to date.
type Snapshot struct {
	Date       string `json:"date"`
	Interface  string `json:"interface"`
	CycleStart string `json:"cycle_start"`
	Today      Day    `json:"today"`
	Yesterday  Day    `json:"yesterday"`
	Month      Day    `json:"month"`
}

type state struct {
	Days      map[string]*Day `json:"days"`
	Interface string          `json:"interface"`
	HaveLast  bool            `json:"have_last"`
	LastRx    uint64          `json:"last_rx"`
	LastTx    uint64          `json:"last_tx"`
}

// Store is safe for concurrent use.
type Store struct {
	mu         sync.Mutex
	path       string
	iface      string
	loc        *time.Location
	billingDay int
	st         state
	dirty      bool
	read       func(iface string) (rx, tx uint64, err error)
}

// Open loads the state file, or starts empty if there is none. An unreadable
// file is moved aside, so the service still starts.
func Open(path, iface string, loc *time.Location) (*Store, error) {
	s := &Store{path: path, iface: iface, loc: loc, read: readProcNetDev, billingDay: 1}
	s.st.Days = map[string]*Day{}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state file: %w", err)
	}

	var st state
	if err := json.Unmarshal(data, &st); err != nil {
		bad := path + ".corrupt"
		_ = os.Rename(path, bad)
		logging.Logf("warn", "state file was unreadable; moved to %s and starting empty: %v", bad, err)
		return s, nil
	}
	if st.Days == nil {
		st.Days = map[string]*Day{}
	}
	s.st = st
	return s, nil
}

func (s *Store) key(t time.Time) string {
	return t.In(s.loc).Format("2006-01-02")
}

// day returns the entry for t's date, creating it if needed. Caller holds mu.
func (s *Store) day(t time.Time) *Day {
	k := s.key(t)
	d := s.st.Days[k]
	if d == nil {
		d = &Day{}
		s.st.Days[k] = d
	}
	return d
}

func (s *Store) add(f func(*Day)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(s.day(time.Now()))
	s.dirty = true
}

// AddConn counts one bridged connection.
func (s *Store) AddConn() { s.add(func(d *Day) { d.BridgeConns++ }) }

// AddUp counts n bytes sent toward the far side.
func (s *Store) AddUp(n int64) { s.add(func(d *Day) { d.BridgeUp += n }) }

// AddDown counts n bytes received from the far side.
func (s *Store) AddDown(n int64) { s.add(func(d *Day) { d.BridgeDown += n }) }

// Sample reads the interface counters and adds the change since the last sample
// to now's day. If the counters went down (a reboot), the new value is counted from zero.
func (s *Store) Sample(now time.Time) error {
	rx, tx, err := s.read(s.iface)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.st.Interface != s.iface {
		s.st.Interface = s.iface
		s.st.HaveLast = false
	}
	if s.st.HaveLast {
		d := s.day(now)
		d.NICRx += delta(rx, s.st.LastRx)
		d.NICTx += delta(tx, s.st.LastTx)
	}
	s.st.LastRx, s.st.LastTx, s.st.HaveLast = rx, tx, true
	s.dirty = true
	return nil
}

func delta(cur, last uint64) int64 {
	if cur >= last {
		return int64(cur - last)
	}
	return int64(cur)
}

// Save writes the state file if anything changed. The write is atomic:
// a temporary file is renamed over the old one.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.dirty {
		return nil
	}
	s.prune()

	data, err := json.MarshalIndent(s.st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o640); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	s.dirty = false
	return nil
}

// prune drops the oldest days beyond keepDays. Date keys sort chronologically.
func (s *Store) prune() {
	if len(s.st.Days) <= keepDays {
		return
	}
	keys := make([]string, 0, len(s.st.Days))
	for k := range s.st.Days {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys[:len(keys)-keepDays] {
		delete(s.st.Days, k)
	}
}

// Snapshot returns today's, yesterday's, and current-cycle totals as of now.
// Month holds the billing cycle totals, which start on the billing day.
func (s *Store) Snapshot(now time.Time) Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	today := s.key(now)
	yesterday := s.key(now.AddDate(0, 0, -1))
	cycle := CycleStart(now, s.billingDay, s.loc)

	snap := Snapshot{Date: today, Interface: s.iface, CycleStart: cycle}
	if d := s.st.Days[today]; d != nil {
		snap.Today = *d
	}
	if d := s.st.Days[yesterday]; d != nil {
		snap.Yesterday = *d
	}
	for k, d := range s.st.Days {
		if k >= cycle && k <= today {
			snap.Month.add(*d)
		}
	}
	return snap
}

// parseNetDev returns the received and sent byte counters for iface.
func parseNetDev(data []byte, iface string) (uint64, uint64, error) {
	for _, line := range strings.Split(string(data), "\n") {
		name, rest, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(name) != iface {
			continue
		}
		f := strings.Fields(rest)
		if len(f) < 9 {
			return 0, 0, fmt.Errorf("unexpected /proc/net/dev line for %s", iface)
		}
		rx, err := strconv.ParseUint(f[0], 10, 64)
		if err != nil {
			return 0, 0, err
		}
		tx, err := strconv.ParseUint(f[8], 10, 64)
		if err != nil {
			return 0, 0, err
		}
		return rx, tx, nil
	}
	return 0, 0, fmt.Errorf("interface %q not found in /proc/net/dev", iface)
}

func readProcNetDev(iface string) (uint64, uint64, error) {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return 0, 0, fmt.Errorf("read /proc/net/dev (Linux only): %w", err)
	}
	return parseNetDev(data, iface)
}

// SetBillingDay sets the day of the month when a new billing cycle starts (1 to 28).
// Zero or an out-of-range value falls back to 1.
func (s *Store) SetBillingDay(day int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if day < 1 || day > 28 {
		day = 1
	}
	s.billingDay = day
}

// CycleStart returns the first date of the billing cycle that contains now,
// formatted as "2006-01-02" in loc.
func CycleStart(now time.Time, billingDay int, loc *time.Location) string {
	if billingDay < 1 {
		billingDay = 1
	}
	y, m, d := now.In(loc).Date()
	if d < billingDay {
		m-- // time.Date normalizes month 0 to December of the previous year
	}
	return time.Date(y, m, billingDay, 0, 0, 0, 0, loc).Format("2006-01-02")
}
