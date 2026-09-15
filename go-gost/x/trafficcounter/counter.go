// Package trafficcounter accounts per-rule ingress and egress bytes with nftables counters.
package trafficcounter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

type Metadata struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ListenPort int    `json:"listenPort"`
}

type Sample struct {
	ID           string
	Name         string
	Ingress      int64
	Egress       int64
	ingressTotal uint64
	egressTotal  uint64
}

type Runner func(context.Context, ...string) ([]byte, error)

type totals struct{ ingress, egress uint64 }
type persistedTotals struct {
	Ingress uint64 `json:"ingress"`
	Egress  uint64 `json:"egress"`
}

type Manager struct {
	mu          sync.Mutex
	Dir, Binary string
	Run         Runner
	baseline    map[string]totals
}

var Default = &Manager{
	Dir:    "/var/lib/flux-panel/counters",
	Binary: "/usr/sbin/nft",
	Run: func(ctx context.Context, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, "/usr/sbin/nft", args...).CombinedOutput()
	},
	baseline: make(map[string]totals),
}

var idPattern = regexp.MustCompile(`^[1-9][0-9]{0,17}$`)
var namePattern = regexp.MustCompile(`^[1-9][0-9]{0,17}_[0-9]{1,18}_[0-9]{1,18}$`)

func validate(meta Metadata) error {
	if !idPattern.MatchString(meta.ID) || !namePattern.MatchString(meta.Name) || meta.ListenPort < 1 || meta.ListenPort > 65535 {
		return errors.New("invalid traffic counter metadata")
	}
	return nil
}

func table(id string) string { return "flux_count_" + id }

func (m *Manager) call(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := m.Run(ctx, args...)
	if err != nil {
		return out, fmt.Errorf("nft: %w: %.512s", err, out)
	}
	return out, nil
}

func (m *Manager) Apply(meta Metadata) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.apply(meta)
}

func (m *Manager) apply(meta Metadata) error {
	if err := validate(meta); err != nil {
		return err
	}
	if _, err := os.Stat(m.Binary); err != nil {
		return errors.New("nftables is required for traffic accounting")
	}
	if m.baseline == nil {
		m.baseline = make(map[string]totals)
	}
	if err := os.MkdirAll(m.Dir, 0700); err != nil {
		return err
	}
	conf := fmt.Sprintf("table inet %s {\n chain ingress { type filter hook prerouting priority -301; policy accept; tcp dport %d counter comment \"ingress-tcp\"; udp dport %d counter comment \"ingress-udp\"; }\n chain egress { type filter hook postrouting priority 101; policy accept; tcp sport %d counter comment \"egress-tcp\"; udp sport %d counter comment \"egress-udp\"; }\n}\n", table(meta.ID), meta.ListenPort, meta.ListenPort, meta.ListenPort, meta.ListenPort)
	confPath := filepath.Join(m.Dir, meta.ID+".nft")
	metaPath := filepath.Join(m.Dir, meta.ID+".json")
	data, _ := json.Marshal(meta)
	if err := os.WriteFile(confPath, []byte(conf), 0600); err != nil {
		return err
	}
	if err := os.WriteFile(metaPath, data, 0600); err != nil {
		return err
	}
	_, _ = m.call("delete", "table", "inet", table(meta.ID))
	if _, err := m.call("-f", confPath); err != nil {
		return err
	}
	m.baseline[meta.ID] = totals{}
	if err := m.writeAck(meta.ID, totals{}); err != nil {
		return err
	}
	return nil
}

func (m *Manager) Pause(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !idPattern.MatchString(id) {
		return errors.New("invalid rule ID")
	}
	_, err := m.call("delete", "table", "inet", table(id))
	delete(m.baseline, id)
	return err
}

func (m *Manager) Resume(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	meta, err := m.readMeta(id)
	if err != nil {
		return err
	}
	return m.apply(meta)
}

func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !idPattern.MatchString(id) {
		return errors.New("invalid rule ID")
	}
	_, _ = m.call("delete", "table", "inet", table(id))
	delete(m.baseline, id)
	for _, ext := range []string{".nft", ".json", ".ack"} {
		_ = os.Remove(filepath.Join(m.Dir, id+ext))
	}
	return nil
}

func (m *Manager) readMeta(id string) (Metadata, error) {
	if !idPattern.MatchString(id) {
		return Metadata{}, errors.New("invalid rule ID")
	}
	b, err := os.ReadFile(filepath.Join(m.Dir, id+".json"))
	if err != nil {
		return Metadata{}, err
	}
	var meta Metadata
	if err := json.Unmarshal(b, &meta); err != nil {
		return Metadata{}, err
	}
	return meta, validate(meta)
}

func (m *Manager) Samples() ([]Sample, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.baseline == nil {
		m.baseline = make(map[string]totals)
	}
	files, err := filepath.Glob(filepath.Join(m.Dir, "*.json"))
	if err != nil {
		return nil, err
	}
	var samples []Sample
	for _, file := range files {
		id := filepath.Base(file[:len(file)-len(filepath.Ext(file))])
		meta, err := m.readMeta(id)
		if err != nil {
			continue
		}
		out, err := m.call("-j", "list", "table", "inet", table(id))
		if err != nil {
			continue
		}
		current, err := parseTotals(out)
		if err != nil {
			continue
		}
		previous, known := m.baseline[id]
		if !known {
			previous, known = m.readAck(id)
			if !known {
				m.baseline[id] = current
				_ = m.writeAck(id, current)
				continue
			}
			m.baseline[id] = previous
		}
		samples = append(samples, Sample{ID: id, Name: meta.Name,
			Ingress: int64(delta(current.ingress, previous.ingress)), Egress: int64(delta(current.egress, previous.egress)),
			ingressTotal: current.ingress, egressTotal: current.egress})
	}
	return samples, nil
}

func (m *Manager) Ack(sample Sample) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.baseline == nil {
		m.baseline = make(map[string]totals)
	}
	value := totals{sample.ingressTotal, sample.egressTotal}
	m.baseline[sample.ID] = value
	if err := m.writeAck(sample.ID, value); err != nil {
		fmt.Printf("failed to persist traffic acknowledgement for rule %s: %v\n", sample.ID, err)
	}
}

func (m *Manager) writeAck(id string, value totals) error {
	data, err := json.Marshal(persistedTotals{Ingress: value.ingress, Egress: value.egress})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(m.Dir, id+".ack"), data, 0600)
}

func (m *Manager) readAck(id string) (totals, bool) {
	data, err := os.ReadFile(filepath.Join(m.Dir, id+".ack"))
	if err != nil {
		return totals{}, false
	}
	var value persistedTotals
	if json.Unmarshal(data, &value) != nil {
		return totals{}, false
	}
	return totals{ingress: value.Ingress, egress: value.Egress}, true
}

func delta(current, previous uint64) uint64 {
	if current < previous {
		return current
	}
	return current - previous
}

func parseTotals(data []byte) (totals, error) {
	var root interface{}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		return totals{}, err
	}
	var result totals
	var walk func(interface{}, string)
	walk = func(value interface{}, direction string) {
		switch v := value.(type) {
		case map[string]interface{}:
			if c, ok := v["comment"].(string); ok {
				if len(c) >= 7 && c[:7] == "ingress" {
					direction = "ingress"
				}
				if len(c) >= 6 && c[:6] == "egress" {
					direction = "egress"
				}
			}
			if counter, ok := v["counter"].(map[string]interface{}); ok {
				if number, ok := counter["bytes"].(json.Number); ok {
					bytesValue, err := number.Int64()
					if err == nil && bytesValue >= 0 {
						if direction == "ingress" {
							result.ingress += uint64(bytesValue)
						}
						if direction == "egress" {
							result.egress += uint64(bytesValue)
						}
					}
				}
			}
			for _, child := range v {
				walk(child, direction)
			}
		case []interface{}:
			for _, child := range v {
				walk(child, direction)
			}
		}
	}
	walk(root, "")
	return result, nil
}
