package realm

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestConfig(t *testing.T) {
	data, err := Config(Request{ID: "12", ListenPort: 8443, Remote: "[2001:db8::1]:443"})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Network   map[string]interface{} `json:"network"`
		Endpoints []map[string]string    `json:"endpoints"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Endpoints) != 1 || got.Endpoints[0]["listen"] != "0.0.0.0:8443" || got.Endpoints[0]["remote"] != "[2001:db8::1]:443" {
		t.Fatalf("unexpected config: %s", data)
	}
}

func TestValidateRejectsUntrustedValues(t *testing.T) {
	cases := []Request{
		{ID: "../1", ListenPort: 80, Remote: "example.com:443"},
		{ID: "1", ListenPort: 0, Remote: "example.com:443"},
		{ID: "1", ListenPort: 80, Remote: "example.com:0"},
		{ID: "1", ListenPort: 80, Remote: "bad host:443"},
		{ID: "1", ListenPort: 80, Remote: "bad..host:443"},
		{ID: "1", ListenPort: 80, Remote: "-bad.example:443"},
	}
	for _, tc := range cases {
		if err := Validate(tc); err == nil {
			t.Fatalf("accepted invalid request: %+v", tc)
		}
	}
}

func testManager(t *testing.T, run Runner) (*Manager, string, string) {
	t.Helper()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	unitDir := filepath.Join(root, "units")
	if err := os.MkdirAll(unitDir, 0755); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "realm")
	if err := os.WriteFile(binary, []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}
	return &Manager{ConfigDir: configDir, UnitDir: unitDir, Binary: binary, Run: run}, configDir, unitDir
}

func TestApplyAndDelete(t *testing.T) {
	var calls [][]string
	m, configDir, unitDir := testManager(t, func(_ context.Context, args ...string) error {
		calls = append(calls, append([]string(nil), args...))
		return nil
	})
	r := Request{ID: "42", ListenPort: 2443, Remote: "example.com:443"}
	if err := m.Execute("ApplyRealm", r); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(configDir, "42.json")); err != nil {
		t.Fatal(err)
	}
	unitData, err := os.ReadFile(filepath.Join(unitDir, "flux-realm-42.service"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(unitData), m.Binary) || !strings.Contains(string(unitData), filepath.Join(configDir, "42.json")) {
		t.Fatalf("unit misses managed paths: %s", unitData)
	}
	wantPrefix := [][]string{{"daemon-reload"}, {"enable", "flux-realm-42.service"}, {"restart", "flux-realm-42.service"}, {"is-active", "--quiet", "flux-realm-42.service"}}
	if !reflect.DeepEqual(calls, wantPrefix) {
		t.Fatalf("calls = %#v", calls)
	}
	if err := m.Execute("DeleteRealm", Request{ID: "42"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(configDir, "42.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("config remains: %v", err)
	}
	if _, err := os.Stat(filepath.Join(unitDir, "flux-realm-42.service")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unit remains: %v", err)
	}
}

func TestFailedFirstApplyRemovesPartialState(t *testing.T) {
	m, configDir, unitDir := testManager(t, func(_ context.Context, args ...string) error {
		if len(args) > 0 && args[0] == "restart" {
			return errors.New("restart failed")
		}
		return nil
	})
	err := m.Execute("ApplyRealm", Request{ID: "7", ListenPort: 9000, Remote: "127.0.0.1:9001"})
	if err == nil || !strings.Contains(err.Error(), "previous configuration restored") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(configDir, "7.json")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("config remains: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(unitDir, "flux-realm-7.service")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("unit remains: %v", statErr)
	}
}
