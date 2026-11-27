package checks

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
)

// Group is a named list of addresses, shown as one section of the report.
type Group struct {
	Name    string   `json:"name"`
	Targets []string `json:"targets"`
}

// TargetFile is the JSON file that lists what to check.
type TargetFile struct {
	Groups []Group `json:"groups"`
}

// LoadTargets reads and validates the target file at path.
func LoadTargets(path string) (TargetFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return TargetFile{}, fmt.Errorf("read targets file: %w", err)
	}
	return ParseTargets(data)
}

// ParseTargets validates the JSON content of a target file.
func ParseTargets(data []byte) (TargetFile, error) {
	var tf TargetFile
	if err := json.Unmarshal(data, &tf); err != nil {
		return TargetFile{}, fmt.Errorf("decode targets: %w", err)
	}
	total := 0
	for _, g := range tf.Groups {
		if strings.TrimSpace(g.Name) == "" {
			return TargetFile{}, errors.New("every group needs a name")
		}
		for _, ip := range g.Targets {
			if net.ParseIP(ip) == nil {
				return TargetFile{}, fmt.Errorf("group %q: %q is not an IP address", g.Name, ip)
			}
			total++
		}
	}
	if total == 0 {
		return TargetFile{}, errors.New("targets file has no addresses")
	}
	return tf, nil
}
