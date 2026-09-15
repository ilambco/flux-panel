// Package forwarder manages allow-listed host forwarding runtimes without a shell.
package forwarder

import (
	"context"
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

	"github.com/go-gost/x/trafficcounter"
)

type Request struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Engine     string `json:"engine"`
	ListenPort int    `json:"listenPort"`
	Remote     string `json:"remote"`
}

type Runner func(context.Context, ...string) error

type Manager struct {
	mu                sync.Mutex
	StateDir, UnitDir string
	Run               Runner
	CheckEnvironment  func() error
	Settle            func()
}

var Default = &Manager{
	StateDir: "/var/lib/flux-panel/forwarder",
	UnitDir:  "/etc/systemd/system",
	Run: func(ctx context.Context, args ...string) error {
		out, err := exec.CommandContext(ctx, "/usr/bin/systemctl", args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("systemctl: %w: %.512s", err, out)
		}
		return nil
	},
	CheckEnvironment: func() error {
		if runtime.GOOS != "linux" {
			return errors.New("external forwarders require Linux with systemd")
		}
		if _, err := os.Stat("/run/systemd/system"); err != nil {
			return errors.New("external forwarders require host systemd")
		}
		return nil
	},
	Settle: func() { time.Sleep(400 * time.Millisecond) },
}

var idPattern = regexp.MustCompile(`^[1-9][0-9]{0,17}$`)
var namePattern = regexp.MustCompile(`^[1-9][0-9]{0,17}_[0-9]{1,18}_[0-9]{1,18}$`)

func IsExternal(engine string) bool {
	switch strings.ToLower(strings.TrimSpace(engine)) {
	case "iptables", "nftables", "socat", "nginx":
		return true
	default:
		return false
	}
}

func Validate(r Request, full bool) error {
	r.Engine = strings.ToLower(strings.TrimSpace(r.Engine))
	if !idPattern.MatchString(r.ID) || !IsExternal(r.Engine) {
		return errors.New("invalid rule ID or forwarding engine")
	}
	if !full {
		return nil
	}
	if !namePattern.MatchString(r.Name) {
		return errors.New("invalid service name")
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
	if (r.Engine == "iptables" || r.Engine == "nftables") && net.ParseIP(host) == nil {
		return errors.New("iptables and nftables require an IP address as target")
	}
	if strings.ContainsAny(host, "\r\n\t ;$`\\\"") {
		return errors.New("invalid remote host")
	}
	return nil
}

type artifact struct {
	path string
	data []byte
	mode os.FileMode
	unit string
}

func (m *Manager) Execute(action string, r Request) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r.Engine = strings.ToLower(strings.TrimSpace(r.Engine))
	if err := Validate(r, action == "ApplyForwarder"); err != nil {
		return err
	}
	if m.CheckEnvironment != nil {
		if err := m.CheckEnvironment(); err != nil {
			return err
		}
	}
	units := unitNames(r.Engine, r.ID)
	switch action {
	case "ApplyForwarder":
		return m.apply(r)
	case "PauseForwarder":
		if err := m.control("disable", units, true); err != nil {
			return err
		}
		return trafficcounter.Default.Pause(r.ID)
	case "ResumeForwarder":
		if err := trafficcounter.Default.Resume(r.ID); err != nil {
			return err
		}
		if err := m.control("enable", units, true); err != nil {
			_ = trafficcounter.Default.Pause(r.ID)
			return err
		}
		if err := m.healthy(units); err != nil {
			_ = trafficcounter.Default.Pause(r.ID)
			return err
		}
		return nil
	case "DeleteForwarder":
		if err := m.control("disable", units, false); err != nil {
			return err
		}
		for _, unit := range units {
			_ = os.Remove(filepath.Join(m.UnitDir, unit))
		}
		_ = os.Remove(filepath.Join(m.StateDir, r.ID+"."+r.Engine))
		_ = os.Remove(filepath.Join(m.StateDir, r.ID+".meta"))
		if err := trafficcounter.Default.Delete(r.ID); err != nil {
			return err
		}
		return m.run("daemon-reload")
	default:
		return errors.New("unsupported forwarder action")
	}
}

func unitNames(engine, id string) []string {
	if engine == "socat" {
		return []string{"flux-socat-" + id + "-tcp.service", "flux-socat-" + id + "-udp.service"}
	}
	return []string{"flux-" + engine + "-" + id + ".service"}
}

func (m *Manager) run(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return m.Run(ctx, args...)
}

func (m *Manager) control(verb string, units []string, strict bool) error {
	for _, unit := range units {
		if strict {
			if _, err := os.Stat(filepath.Join(m.UnitDir, unit)); err != nil {
				return fmt.Errorf("forwarder rule missing: %w", err)
			}
		}
		if err := m.run(verb, "--now", unit); err != nil && strict {
			return err
		}
	}
	return nil
}

func (m *Manager) healthy(units []string) error {
	if m.Settle != nil {
		m.Settle()
	}
	for _, unit := range units {
		if err := m.run("is-active", "--quiet", unit); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) apply(r Request) error {
	arts, err := m.artifacts(r)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(m.StateDir, 0700); err != nil {
		return err
	}
	for _, a := range arts {
		if err := atomicWrite(a.path, a.data, a.mode); err != nil {
			return err
		}
	}
	meta := []byte(r.Name + "\n" + strconv.Itoa(r.ListenPort) + "\n" + r.Engine + "\n")
	if err := atomicWrite(filepath.Join(m.StateDir, r.ID+".meta"), meta, 0600); err != nil {
		return err
	}
	if err := m.run("daemon-reload"); err != nil {
		return err
	}
	units := unitNames(r.Engine, r.ID)
	for _, unit := range units {
		if err := m.run("enable", unit); err != nil {
			return err
		}
		if err := m.run("restart", unit); err != nil {
			return err
		}
	}
	if err := m.healthy(units); err != nil {
		return err
	}
	if err := trafficcounter.Default.Apply(trafficcounter.Metadata{ID: r.ID, Name: r.Name, ListenPort: r.ListenPort}); err != nil {
		_ = m.control("disable", units, false)
		return err
	}
	return nil
}

func (m *Manager) artifacts(r Request) ([]artifact, error) {
	host, port, _ := net.SplitHostPort(r.Remote)
	state := filepath.Join(m.StateDir, r.ID+"."+r.Engine)
	desc := "Flux Panel " + r.Engine + " rule " + r.ID
	unit := func(execStart, execStop, unitType string) []byte {
		stop := ""
		if execStop != "" {
			stop = "ExecStop=" + execStop + "\n"
		}
		remain := ""
		if unitType == "oneshot" {
			remain = "RemainAfterExit=yes\n"
		}
		return []byte(fmt.Sprintf("[Unit]\nDescription=%s\nAfter=network-online.target\nWants=network-online.target\n[Service]\nType=%s\n%sExecStart=%s\n%sRestart=%s\nNoNewPrivileges=true\n[Install]\nWantedBy=multi-user.target\n", desc, unitType, remain, execStart, stop, map[bool]string{true: "no", false: "on-failure"}[unitType == "oneshot"]))
	}
	units := unitNames(r.Engine, r.ID)
	switch r.Engine {
	case "socat":
		base := func(proto string) string {
			return fmt.Sprintf("/usr/bin/socat %s-LISTEN:%d,reuseaddr,fork %s:%s", proto, r.ListenPort, proto, r.Remote)
		}
		return []artifact{
			{filepath.Join(m.UnitDir, units[0]), unit(base("TCP"), "", "simple"), 0644, units[0]},
			{filepath.Join(m.UnitDir, units[1]), unit(base("UDP"), "", "simple"), 0644, units[1]},
		}, nil
	case "nginx":
		conf := []byte(fmt.Sprintf("include /etc/nginx/modules-enabled/*.conf;\nworker_processes 1;\nerror_log stderr;\npid /run/flux-nginx-%s.pid;\nevents {}\nstream {\n server { listen %d; proxy_pass %s; }\n server { listen %d udp reuseport; proxy_pass %s; }\n}\n", r.ID, r.ListenPort, r.Remote, r.ListenPort, r.Remote))
		return []artifact{{state, conf, 0600, ""}, {filepath.Join(m.UnitDir, units[0]), unit("/usr/sbin/nginx -c "+state+" -g daemon\\ off;", "", "simple"), 0644, units[0]}}, nil
	case "nftables":
		if net.ParseIP(host).To4() == nil {
			return nil, errors.New("nftables runtime currently requires IPv4")
		}
		conf := []byte(fmt.Sprintf("table ip flux_%s {\n chain pre { type nat hook prerouting priority dstnat; policy accept; tcp dport %d dnat to %s:%s; udp dport %d dnat to %s:%s; }\n chain post { type nat hook postrouting priority srcnat; policy accept; ip daddr %s masquerade; }\n chain fwd { type filter hook forward priority filter; policy accept; }\n}\n", r.ID, r.ListenPort, host, port, r.ListenPort, host, port, host))
		return []artifact{{state, conf, 0600, ""}, {filepath.Join(m.UnitDir, units[0]), unit("/usr/sbin/nft -f "+state, "/usr/sbin/nft delete table ip flux_"+r.ID, "oneshot"), 0644, units[0]}}, nil
	case "iptables":
		if net.ParseIP(host).To4() == nil {
			return nil, errors.New("iptables runtime currently requires IPv4")
		}
		lines := fmt.Sprintf("[Unit]\nDescription=%s\nAfter=network-online.target\n[Service]\nType=oneshot\nRemainAfterExit=yes\nExecStart=/usr/sbin/iptables -t nat -A PREROUTING -p tcp --dport %d -j DNAT --to-destination %s:%s\nExecStart=/usr/sbin/iptables -t nat -A PREROUTING -p udp --dport %d -j DNAT --to-destination %s:%s\nExecStart=/usr/sbin/iptables -t nat -A POSTROUTING -d %s -j MASQUERADE\nExecStop=/usr/sbin/iptables -t nat -D PREROUTING -p tcp --dport %d -j DNAT --to-destination %s:%s\nExecStop=/usr/sbin/iptables -t nat -D PREROUTING -p udp --dport %d -j DNAT --to-destination %s:%s\nExecStop=/usr/sbin/iptables -t nat -D POSTROUTING -d %s -j MASQUERADE\n[Install]\nWantedBy=multi-user.target\n", desc, r.ListenPort, host, port, r.ListenPort, host, port, host, r.ListenPort, host, port, r.ListenPort, host, port, host)
		return []artifact{{filepath.Join(m.UnitDir, units[0]), []byte(lines), 0644, units[0]}}, nil
	}
	return nil, errors.New("unsupported forwarding engine")
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".forwarder-*")
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
