package forwarder

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-gost/x/trafficcounter"
)

func TestArtifactsCoverAllEngines(t *testing.T) {
	root := t.TempDir()
	m := &Manager{StateDir: filepath.Join(root, "state"), UnitDir: filepath.Join(root, "units")}
	for _, engine := range []string{"iptables", "nftables", "socat", "nginx"} {
		r := Request{ID: "9", Name: "9_1_0", Engine: engine, ListenPort: 8443, Remote: "192.0.2.8:443"}
		arts, err := m.artifacts(r)
		if err != nil {
			t.Fatalf("%s: %v", engine, err)
		}
		if len(arts) == 0 {
			t.Fatalf("%s produced no artifacts", engine)
		}
		var joined string
		for _, a := range arts {
			joined += string(a.data)
		}
		if !strings.Contains(joined, "8443") || !strings.Contains(joined, "192.0.2.8") {
			t.Fatalf("%s config incomplete", engine)
		}
	}
}

func TestApplyPauseResumeDelete(t *testing.T) {
	root := t.TempDir()
	unitDir := filepath.Join(root, "units")
	if err := os.MkdirAll(unitDir, 0755); err != nil {
		t.Fatal(err)
	}
	oldCounter := trafficcounter.Default
	nft := filepath.Join(root, "nft")
	if err := os.WriteFile(nft, []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}
	trafficcounter.Default = &trafficcounter.Manager{Dir: filepath.Join(root, "counters"), Binary: nft, Run: func(_ context.Context, _ ...string) ([]byte, error) { return []byte(`{"nftables":[]}`), nil }}
	t.Cleanup(func() { trafficcounter.Default = oldCounter })
	m := &Manager{StateDir: filepath.Join(root, "state"), UnitDir: unitDir, Run: func(_ context.Context, _ ...string) error { return nil }}
	r := Request{ID: "9", Name: "9_1_0", Engine: "nginx", ListenPort: 8443, Remote: "example.com:443"}
	if err := m.Execute("ApplyForwarder", r); err != nil {
		t.Fatal(err)
	}
	if err := m.Execute("PauseForwarder", Request{ID: "9", Engine: "nginx"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Execute("ResumeForwarder", Request{ID: "9", Engine: "nginx"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Execute("DeleteForwarder", Request{ID: "9", Engine: "nginx"}); err != nil {
		t.Fatal(err)
	}
}
