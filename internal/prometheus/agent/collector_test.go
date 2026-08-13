package agentmetrics

import (
	"strings"
	"testing"

	"git.g3e.fr/syonad/two/internal/state"
	"git.g3e.fr/syonad/two/pkg/db/kv"

	"github.com/dgraph-io/badger/v4"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func newTestDB(t *testing.T) *badger.DB {
	t.Helper()
	db := kv.InitDB(kv.Config{Path: t.TempDir()}, false)
	t.Cleanup(func() { db.Close() })
	return db
}

// collect exécute une collecte et retourne, par nom de métrique, la valeur de
// chaque série indexée par son label state.
func collect(t *testing.T, c *AgentCollector) map[string]map[string]float64 {
	t.Helper()
	ch := make(chan prometheus.Metric, 64)
	c.Collect(ch)
	close(ch)

	out := map[string]map[string]float64{}
	for m := range ch {
		var pb dto.Metric
		if err := m.Write(&pb); err != nil {
			t.Fatalf("write metric: %v", err)
		}
		// Le fqName n'est pas exposé directement : il figure dans la
		// représentation textuelle du Desc.
		desc := m.Desc().String()
		var name string
		switch {
		case strings.Contains(desc, "syonad_vpcs_total"):
			name = "vpcs"
		case strings.Contains(desc, "syonad_subnets_total"):
			name = "subnets"
		default:
			t.Fatalf("métrique inattendue : %s", desc)
		}
		if out[name] == nil {
			out[name] = map[string]float64{}
		}
		for _, l := range pb.GetLabel() {
			if l.GetName() == "state" {
				out[name][l.GetValue()] = pb.GetGauge().GetValue()
			}
		}
	}
	return out
}

func TestCollector_CountsByState(t *testing.T) {
	db := newTestDB(t)
	for resource, s := range map[string]state.State{
		"vpc/vpc-1":   state.Running,
		"vpc/vpc-2":   state.Running,
		"vpc/vpc-3":   state.Error,
		"subnet/sn-1": state.Creating,
	} {
		if err := state.Set(db, resource, s); err != nil {
			t.Fatalf("préparation du test : %v", err)
		}
	}

	got := collect(t, NewAgentCollector(db))
	wantVPC := map[string]float64{"creating": 0, "running": 2, "error": 1, "deleting": 0, "deleted": 0}
	for s, want := range wantVPC {
		if got["vpcs"][s] != want {
			t.Errorf("vpcs{state=%q} = %v, attendu %v", s, got["vpcs"][s], want)
		}
	}
	if got["subnets"]["creating"] != 1 {
		t.Errorf("subnets{state=creating} = %v, attendu 1", got["subnets"]["creating"])
	}
}

// Une série par état doit être émise même à zéro : une métrique qui disparaît
// côté Prometheus casse les alertes qui s'en servent.
func TestCollector_EmitsEveryState(t *testing.T) {
	got := collect(t, NewAgentCollector(newTestDB(t)))
	for _, family := range []string{"vpcs", "subnets"} {
		if len(got[family]) != len(state.All()) {
			t.Errorf("%s : %d séries, attendu %d", family, len(got[family]), len(state.All()))
		}
		for _, s := range state.All() {
			if _, ok := got[family][string(s)]; !ok {
				t.Errorf("%s : série manquante pour l'état %q", family, s)
			}
		}
	}
}

func TestCollector_IgnoresNonStateKeys(t *testing.T) {
	db := newTestDB(t)
	if err := state.Set(db, "vpc/vpc-1", state.Running); err != nil {
		t.Fatalf("préparation du test : %v", err)
	}
	kv.AddInDB(db, "vpc/vpc-1/cidr", "10.0.0.0/16")

	got := collect(t, NewAgentCollector(db))
	if got["vpcs"]["running"] != 1 {
		t.Errorf("vpcs{state=running} = %v, attendu 1", got["vpcs"]["running"])
	}
}

func TestCollector_Describe(t *testing.T) {
	ch := make(chan *prometheus.Desc, 10)
	NewAgentCollector(newTestDB(t)).Describe(ch)
	close(ch)

	var n int
	for range ch {
		n++
	}
	if n != 2 {
		t.Errorf("%d descripteurs, attendu 2", n)
	}
}
