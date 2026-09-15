// Package realm manages isolated realm services. It does not modify GOST configs.
package realm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Request struct {
	ID         string `json:"id"`
	ListenPort int    `json:"listenPort"`
	Remote     string `json:"remote"`
}

type Runner func(context.Context, ...string) error

type Manager struct {
	mu                         sync.Mutex
	ConfigDir, UnitDir, Binary string
	Run                        Runner
	CheckEnvironment           func() error
	Settle                     func()
}

var Default = &Manager{
	ConfigDir: "/var/lib/flux-panel/realm",
	UnitDir:   "/etc/systemd/system",
	Binary:    "/usr/local/lib/flux-panel/realm",
	Run: func(ctx context.Context, args ...string) error {
		// Arguments come from the fixed action set and numeric rule IDs, never a shell.
		out, err := exec.CommandContext(ctx, "/usr/bin/systemctl", args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("systemctl: %w: %.512s", err, out)
		}
		return nil
	},
	CheckEnvironment: func() error {
		if runtime.GOOS != "linux" {
			return errors.New("realm requires Linux with systemd")
		}
		if _, err := os.Stat("/run/systemd/system"); err != nil {
			return errors.New("realm requires host systemd; container nodes are not supported")
		}
		return nil
	},
	Settle: func() { time.Sleep(400 * time.Millisecond) },
}

var idPattern = regexp.MustCompile(`^[1-9][0-9]{0,17}$`)

func validHostname(host string) bool {
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	if strings.HasSuffix(host, ".") {
		host = strings.TrimSuffix(host, ".")
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
				(char < '0' || char > '9') && char != '-' {
				return false
			}
		}
	}
	return true
}

func Validate(r Request) error {
	if !idPattern.MatchString(r.ID) {
		return errors.New("invalid rule ID")
	}
	if r.ListenPort < 1 || r.ListenPort > 65535 {
		return errors.New("invalid listen port")
	}
	host, port, err := net.SplitHostPort(r.Remote)
	if err != nil || host == "" {
		return errors.New("remote must be one host:port or [IPv6]:port")
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return errors.New("invalid remote port")
	}
	if net.ParseIP(host) == nil && !validHostname(host) {
		return errors.New("invalid remote host")
	}
	return nil
}

func Config(r Request) ([]byte, error) {
	if err := Validate(r); err != nil {
		return nil, err
	}
	return json.MarshalIndent(map[string]interface{}{
		"network":   map[string]interface{}{"no_tcp": false, "use_udp": true},
		"endpoints": []map[string]string{{"listen": net.JoinHostPort("0.0.0.0", strconv.Itoa(r.ListenPort)), "remote": r.Remote}},
	}, "", "  ")
}

// Execute serializes lifecycle operations, including calls from different WS requests.
func (m *Manager) Execute(action string, r Request) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !idPattern.MatchString(r.ID) {
		return errors.New("invalid rule ID")
	}
	if m.CheckEnvironment != nil {
		if err := m.CheckEnvironment(); err != nil {
			return err
		}
	}
	unit := "flux-realm-" + r.ID + ".service"
	conf := filepath.Join(m.ConfigDir, r.ID+".json")
	unitPath := filepath.Join(m.UnitDir, unit)
	switch action {
	case "ApplyRealm":
		return m.apply(r, conf, unitPath, unit)
	case "PauseRealm":
		if _, err := os.Stat(conf); err != nil {
			return fmt.Errorf("realm rule missing: %w", err)
		}
		return m.run("disable", "--now", unit)
	case "ResumeRealm":
		if _, err := os.Stat(conf); err != nil {
			return fmt.Errorf("realm rule missing: %w", err)
		}
		if err := m.run("enable", "--now", unit); err != nil {
			return err
		}
		return m.healthy(unit)
	case "DeleteRealm":
		// Stop before deleting files. An uncertain/failed stop retains recoverable state.
		if _, err := os.Stat(unitPath); errors.Is(err, os.ErrNotExist) {
			if _, err := os.Stat(conf); errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return errors.New("realm unit is missing but config remains; inspect node before retrying")
		} else if err != nil {
			return err
		}
		if err := m.run("disable", "--now", unit); err != nil {
			return err
		}
		if err := os.Remove(unitPath); err != nil {
			return err
		}
		if err := os.Remove(conf); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return m.run("daemon-reload")
	default:
		return errors.New("unsupported realm action")
	}
}

func (m *Manager) run(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	return m.Run(ctx, args...)
}

func (m *Manager) healthy(unit string) error {
	if m.Settle != nil {
		m.Settle()
	}
	return m.run("is-active", "--quiet", unit)
}

func readOptional(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return b, err
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".realm-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}

func restoreFile(path string, data []byte, mode os.FileMode) error {
	if data != nil {
		return atomicWrite(path, data, mode)
	}
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (m *Manager) apply(r Request, conf, unitPath, unit string) error {
	data, err := Config(r)
	if err != nil {
		return err
	}
	info, err := os.Stat(m.Binary)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("realm binary missing; install the verified binary at /usr/local/lib/flux-panel/realm")
	}
	if strings.ContainsAny(m.Binary+m.ConfigDir, "\r\n\"%") {
		return errors.New("invalid managed path")
	}
	if err := os.MkdirAll(m.ConfigDir, 0700); err != nil {
		return err
	}
	oldConfig, err := readOptional(conf)
	if err != nil {
		return err
	}
	oldUnit, err := readOptional(unitPath)
	if err != nil {
		return err
	}
	if (oldConfig == nil) != (oldUnit == nil) {
		return errors.New("incomplete realm state; inspect node before applying")
	}
	wasActive, wasEnabled := false, false
	if oldUnit != nil {
		wasActive = m.run("is-active", "--quiet", unit) == nil
		wasEnabled = m.run("is-enabled", "--quiet", unit) == nil
	}
	unitData := []byte(fmt.Sprintf(`[Unit]
Description=Flux Panel realm rule %s
After=network-online.target
Wants=network-online.target
[Service]
Type=simple
ExecStart="%s" -c "%s"
Restart=on-failure
RestartSec=3
TimeoutStopSec=1
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
[Install]
WantedBy=multi-user.target
`, r.ID, m.Binary, conf))
	// Track each rollback error: never claim restoration when a command failed.
	rollback := func(cause error) error {
		if err := m.run("stop", unit); err != nil {
			return fmt.Errorf("%w; rollback stop failed: %v", cause, err)
		}
		var failures []error
		failures = append(failures, restoreFile(conf, oldConfig, 0600), restoreFile(unitPath, oldUnit, 0644), m.run("daemon-reload"))
		if oldUnit != nil {
			if wasEnabled {
				failures = append(failures, m.run("enable", unit))
			} else {
				failures = append(failures, m.run("disable", unit))
			}
			if wasActive {
				failures = append(failures, m.run("start", unit), m.healthy(unit))
			}
		}
		if restoreErr := errors.Join(failures...); restoreErr != nil {
			return fmt.Errorf("%w; rollback failed: %v", cause, restoreErr)
		}
		return fmt.Errorf("%w; previous configuration restored", cause)
	}
	if err := atomicWrite(conf, data, 0600); err != nil {
		return err
	}
	if err := atomicWrite(unitPath, unitData, 0644); err != nil {
		return errors.Join(err, restoreFile(conf, oldConfig, 0600))
	}
	if err := m.run("daemon-reload"); err != nil {
		return rollback(err)
	}
	if err := m.run("enable", unit); err != nil {
		return rollback(err)
	}
	if err := m.run("restart", unit); err != nil {
		return rollback(err)
	}
	if err := m.healthy(unit); err != nil {
		return rollback(err)
	}
	return nil
}
