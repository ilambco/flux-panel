package trafficcounter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestParseTotalsAndDelta(t *testing.T) {
	data := []byte(`{"nftables":[
		{"rule":{"comment":"ingress-tcp","expr":[{"counter":{"packets":2,"bytes":120}}]}},
		{"rule":{"comment":"ingress-udp","expr":[{"counter":{"packets":1,"bytes":30}}]}},
		{"rule":{"comment":"egress-tcp","expr":[{"counter":{"packets":2,"bytes":90}}]}}
	]}`)
	got, err := parseTotals(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.ingress != 150 || got.egress != 90 {
		t.Fatalf("totals = %+v", got)
	}
	if delta(10, 15) != 10 || delta(20, 15) != 5 {
		t.Fatal("counter reset delta is incorrect")
	}
}

func TestAcknowledgementSurvivesAgentRestart(t *testing.T) {
	root := t.TempDir()
	nft := filepath.Join(root, "nft")
	if err := os.WriteFile(nft, []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}
	ingress, egress := uint64(100), uint64(80)
	runner := func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "-j" {
			return []byte(fmt.Sprintf(`{"nftables":[{"rule":{"comment":"ingress-tcp","expr":[{"counter":{"bytes":%d}}]}},{"rule":{"comment":"egress-tcp","expr":[{"counter":{"bytes":%d}}]}}]}`, ingress, egress)), nil
		}
		return nil, nil
	}
	m := &Manager{Dir: root, Binary: nft, Run: runner}
	if err := m.Apply(Metadata{ID: "3", Name: "3_1_0", ListenPort: 443}); err != nil {
		t.Fatal(err)
	}
	samples, err := m.Samples()
	if err != nil || len(samples) != 1 {
		t.Fatalf("first sample: %v, %+v", err, samples)
	}
	m.Ack(samples[0])

	ingress, egress = 130, 95
	restarted := &Manager{Dir: root, Binary: nft, Run: runner}
	samples, err = restarted.Samples()
	if err != nil || len(samples) != 1 {
		t.Fatalf("restart sample: %v, %+v", err, samples)
	}
	if samples[0].Ingress != 30 || samples[0].Egress != 15 {
		t.Fatalf("restart delta: %+v", samples[0])
	}
}

func TestValidate(t *testing.T) {
	if err := validate(Metadata{ID: "12", Name: "12_1_0", ListenPort: 443}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []Metadata{{ID: "../1", Name: "1_1_0", ListenPort: 1}, {ID: "1", Name: "bad", ListenPort: 1}, {ID: "1", Name: "1_1_0", ListenPort: 0}} {
		if validate(tc) == nil {
			t.Fatalf("accepted invalid metadata: %+v", tc)
		}
	}
}
