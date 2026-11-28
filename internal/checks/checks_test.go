package checks

import (
	"context"
	"encoding/binary"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

var zone = time.FixedZone("TEST", 3*3600+1800)

func TestParseTargets(t *testing.T) {
	good := []byte(`{"groups":[{"name":"Global","targets":["1.1.1.1","8.8.8.8"]}]}`)
	tf, err := ParseTargets(good)
	if err != nil {
		t.Fatal(err)
	}
	if len(tf.Groups) != 1 || len(tf.Groups[0].Targets) != 2 {
		t.Fatalf("unexpected targets: %+v", tf)
	}

	bad := map[string]string{
		"bad ip":   `{"groups":[{"name":"x","targets":["not-an-ip"]}]}`,
		"no name":  `{"groups":[{"name":"","targets":["1.1.1.1"]}]}`,
		"empty":    `{"groups":[]}`,
		"bad json": `{"groups":`,
	}
	for name, input := range bad {
		if _, err := ParseTargets([]byte(input)); err == nil {
			t.Fatalf("%s: expected an error", name)
		}
	}
}

func TestParsePingRTT(t *testing.T) {
	ok := []string{
		"64 bytes from 1.1.1.1: icmp_seq=1 ttl=57 time=12.3 ms",
		"64 bytes from 1.1.1.1: icmp_seq=1 ttl=57 time<1 ms",
	}
	for _, s := range ok {
		if _, got := parsePingRTT([]byte(s)); !got {
			t.Fatalf("expected an RTT from %q", s)
		}
	}
	timeout := "1 packets transmitted, 0 received, 100% packet loss"
	if _, got := parsePingRTT([]byte(timeout)); got {
		t.Fatal("no RTT should be found in a timeout")
	}
}

func TestBuildQuery(t *testing.T) {
	q := buildQuery(0x1234, "example.com")
	if binary.BigEndian.Uint16(q[0:2]) != 0x1234 {
		t.Fatal("transaction ID not set")
	}
	if binary.BigEndian.Uint16(q[4:6]) != 1 {
		t.Fatal("question count should be 1")
	}
	if q[12] != 7 || string(q[13:20]) != "example" {
		t.Fatalf("first label wrong: % x", q[12:20])
	}
}

func TestDNSProbeAgainstLocalServer(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()

	dnsPort = strconv.Itoa(pc.LocalAddr().(*net.UDPAddr).Port)
	t.Cleanup(func() { dnsPort = "53" })

	go func() {
		buf := make([]byte, 512)
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		reply := make([]byte, n)
		copy(reply, buf[:n])
		reply[2] |= 0x80 // mark as a response, keep the transaction ID
		_, _ = pc.WriteTo(reply, addr)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := dnsProbe(ctx, "127.0.0.1"); err != nil {
		t.Fatalf("expected an answer, got: %v", err)
	}
}

func TestDNSProbeNoServer(t *testing.T) {
	// Reserve a port, then close it so nothing answers there.
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	dnsPort = strconv.Itoa(pc.LocalAddr().(*net.UDPAddr).Port)
	pc.Close()
	t.Cleanup(func() { dnsPort = "53" })

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := dnsProbe(ctx, "127.0.0.1"); err == nil {
		t.Fatal("expected an error when no resolver answers")
	}
}

func TestRunProbesKeepsOrderAndStatus(t *testing.T) {
	tf := TargetFile{Groups: []Group{
		{Name: "A", Targets: []string{"1.1.1.1", "2.2.2.2"}},
		{Name: "B", Targets: []string{"3.3.3.3"}},
	}}
	p := Prober{
		Ping: func(_ context.Context, ip string) (time.Duration, error) {
			if ip == "2.2.2.2" {
				return 0, context.DeadlineExceeded
			}
			return 10 * time.Millisecond, nil
		},
		DNS: func(_ context.Context, ip string) (time.Duration, error) {
			if ip == "1.1.1.1" {
				return 5 * time.Millisecond, nil
			}
			return 0, context.DeadlineExceeded
		},
	}

	res := RunProbes(tf, p, 2)
	want := []struct{ ip, status string }{
		{"1.1.1.1", "OK"},
		{"2.2.2.2", "DOWN"},
		{"3.3.3.3", "PARTIAL"},
	}
	for i, w := range want {
		if res[i].IP != w.ip || res[i].Status() != w.status {
			t.Fatalf("result %d = %s %s, want %s %s", i, res[i].IP, res[i].Status(), w.ip, w.status)
		}
	}
}

func TestBuildReport(t *testing.T) {
	res := []Result{
		{Group: "Global", IP: "1.1.1.1", PingOK: true, PingRTT: 12 * time.Millisecond, DNSOK: true, DNSRTT: 14 * time.Millisecond},
		{Group: "Global", IP: "8.8.8.8"},
		{Group: "Example", IP: "192.0.2.10", DNSOK: true, DNSRTT: 30 * time.Millisecond},
	}
	when := time.Date(2026, time.November, 27, 7, 0, 0, 0, zone)
	msg := BuildReport("inside", when, "Asia/Tehran", res)

	for _, want := range []string{
		"connectivity check (inside)", "2026-11-27 07:00", "[Global]", "[Example]",
		"OK", "DOWN", "PARTIAL", "1.1.1.1", "8.8.8.8", "192.0.2.10",
		"ping ok: 1/3, DNS ok: 2/3",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("report missing %q:\n%s", want, msg)
		}
	}
}

func TestNextCheck(t *testing.T) {
	friday := []time.Weekday{time.Friday}
	cases := []struct{ now, want time.Time }{
		{time.Date(2026, 11, 27, 6, 0, 0, 0, zone), time.Date(2026, 11, 27, 7, 0, 0, 0, zone)},
		{time.Date(2026, 11, 27, 7, 0, 0, 0, zone), time.Date(2026, 12, 4, 7, 0, 0, 0, zone)},
		{time.Date(2026, 11, 27, 8, 0, 0, 0, zone), time.Date(2026, 12, 4, 7, 0, 0, 0, zone)},
		{time.Date(2026, 11, 26, 12, 0, 0, 0, zone), time.Date(2026, 11, 27, 7, 0, 0, 0, zone)},
	}
	for _, c := range cases {
		if got := NextCheck(c.now, friday, 7, 0, zone); !got.Equal(c.want) {
			t.Fatalf("NextCheck(%v) = %v, want %v", c.now, got, c.want)
		}
	}
}
