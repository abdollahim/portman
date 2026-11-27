package checks

import (
	"context"
	"encoding/binary"
	"errors"
	"math/rand"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// dnsName is the name asked for in the DNS probe. Any name a resolver answers works.
const dnsName = "example.com"

// dnsPort is 53 in production. Tests point it at a local server.
var dnsPort = "53"

var rttPattern = regexp.MustCompile(`time[=<]([0-9.]+)\s*ms`)

// Result is the outcome of probing one address.
type Result struct {
	Group   string
	IP      string
	PingOK  bool
	PingRTT time.Duration
	DNSOK   bool
	DNSRTT  time.Duration
}

// Status is OK when both checks pass, PARTIAL when one passes, and DOWN otherwise.
func (r Result) Status() string {
	switch {
	case r.PingOK && r.DNSOK:
		return "OK"
	case r.PingOK || r.DNSOK:
		return "PARTIAL"
	default:
		return "DOWN"
	}
}

// Prober runs the two checks. Tests replace these functions.
type Prober struct {
	Ping func(ctx context.Context, ip string) (time.Duration, error)
	DNS  func(ctx context.Context, ip string) (time.Duration, error)
}

// DefaultProber uses the system ping command and a real DNS query over UDP.
func DefaultProber() Prober {
	return Prober{Ping: systemPing, DNS: dnsProbe}
}

// RunProbes checks every address in tf, with at most concurrency checks running at
// once. Results keep the order of the target file.
func RunProbes(tf TargetFile, p Prober, concurrency int) []Result {
	type job struct{ group, ip string }
	var jobs []job
	for _, g := range tf.Groups {
		for _, ip := range g.Targets {
			jobs = append(jobs, job{g.Name, ip})
		}
	}
	if concurrency < 1 {
		concurrency = 1
	}

	results := make([]Result, len(jobs))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i, j := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, j job) {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = probeOne(p, j.group, j.ip)
		}(i, j)
	}
	wg.Wait()
	return results
}

func probeOne(p Prober, group, ip string) Result {
	r := Result{Group: group, IP: ip}

	pingCtx, cancelPing := context.WithTimeout(context.Background(), 5*time.Second)
	if rtt, err := p.Ping(pingCtx, ip); err == nil {
		r.PingOK, r.PingRTT = true, rtt
	}
	cancelPing()

	dnsCtx, cancelDNS := context.WithTimeout(context.Background(), 3*time.Second)
	if rtt, err := p.DNS(dnsCtx, ip); err == nil {
		r.DNSOK, r.DNSRTT = true, rtt
	}
	cancelDNS()
	return r
}

// systemPing sends one ICMP echo with the system ping command.
// The service needs CAP_NET_RAW, which the client unit grants as an ambient capability.
func systemPing(ctx context.Context, ip string) (time.Duration, error) {
	out, err := exec.CommandContext(ctx, "ping", "-c", "1", "-W", "2", ip).CombinedOutput()
	if errors.Is(err, exec.ErrNotFound) {
		return 0, err
	}
	rtt, ok := parsePingRTT(out)
	if !ok {
		return 0, errors.New("no reply")
	}
	return rtt, nil
}

// parsePingRTT reads the round-trip time from ping output, such as "time=12.3 ms".
func parsePingRTT(out []byte) (time.Duration, bool) {
	m := rttPattern.FindSubmatch(out)
	if m == nil {
		return 0, false
	}
	ms, err := strconv.ParseFloat(string(m[1]), 64)
	if err != nil {
		return 0, false
	}
	return time.Duration(ms * float64(time.Millisecond)), true
}

// dnsProbe sends an A query for dnsName and succeeds only when the resolver
// answers with the same transaction ID.
func dnsProbe(ctx context.Context, ip string) (time.Duration, error) {
	id := uint16(rand.Uint32())
	query := buildQuery(id, dnsName)

	var d net.Dialer
	conn, err := d.DialContext(ctx, "udp", net.JoinHostPort(ip, dnsPort))
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}

	start := time.Now()
	if _, err := conn.Write(query); err != nil {
		return 0, err
	}
	buf := make([]byte, 512)
	n, err := conn.Read(buf)
	if err != nil {
		return 0, err
	}
	if !isAnswer(buf[:n], id) {
		return 0, errors.New("unexpected DNS reply")
	}
	return time.Since(start), nil
}

// buildQuery returns a DNS query packet: a header, then one A/IN question.
func buildQuery(id uint16, name string) []byte {
	b := make([]byte, 12, 64)
	binary.BigEndian.PutUint16(b[0:2], id)
	binary.BigEndian.PutUint16(b[2:4], 0x0100) // recursion desired
	binary.BigEndian.PutUint16(b[4:6], 1)      // one question
	for _, label := range strings.Split(name, ".") {
		b = append(b, byte(len(label)))
		b = append(b, label...)
	}
	b = append(b, 0)
	b = binary.BigEndian.AppendUint16(b, 1) // type A
	b = binary.BigEndian.AppendUint16(b, 1) // class IN
	return b
}

// isAnswer reports whether msg is a DNS response (QR bit set) to the query with id.
func isAnswer(msg []byte, id uint16) bool {
	if len(msg) < 12 {
		return false
	}
	return binary.BigEndian.Uint16(msg[0:2]) == id && msg[2]&0x80 != 0
}
