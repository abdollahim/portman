package checks

import (
	"fmt"
	"strings"
	"time"
)

// BuildReport formats the results as plain text, grouped as in the target file.
func BuildReport(label string, when time.Time, zone string, results []Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "connectivity check (%s)\n%s (%s)\n", label, when.Format("2006-01-02 15:04"), zone)

	group := ""
	pingOK, dnsOK := 0, 0
	for _, r := range results {
		if r.Group != group {
			group = r.Group
			fmt.Fprintf(&b, "\n[%s]\n", group)
		}
		fmt.Fprintf(&b, "  %-8s %-15s %s\n", r.Status(), r.IP, r.detail())
		if r.PingOK {
			pingOK++
		}
		if r.DNSOK {
			dnsOK++
		}
	}
	fmt.Fprintf(&b, "\nping ok: %d/%d, DNS ok: %d/%d\n", pingOK, len(results), dnsOK, len(results))
	return b.String()
}

func (r Result) detail() string {
	ping := "ping no reply"
	if r.PingOK {
		ping = "ping " + roundMs(r.PingRTT)
	}
	dns := "DNS no answer"
	if r.DNSOK {
		dns = "DNS " + roundMs(r.DNSRTT)
	}
	return ping + ", " + dns
}

func roundMs(d time.Duration) string {
	return fmt.Sprintf("%d ms", d.Milliseconds())
}
