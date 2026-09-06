package scheduling

import (
	"os"
	"testing"

	"sigs.k8s.io/yaml"
)

// This test parses the committed Kueue Cohort configuration
// (gitops/kueue-resources/cluster-queue.yaml, copied to testdata/ — Go
// tests can't reliably reach outside the module via a relative path, so
// this is a point-in-time copy, not a live read of the real file. If the
// real file changes, this copy needs updating too, or this test will
// pass against stale config while the cluster runs something different.
//
// It exists because the Cohort structure (per-tenant ClusterQueues,
// shared cohortName, borrowing limits, reclaimWithinCohort) was only
// ever proven live on the cluster, never covered by anything that would
// catch a regression if someone edited the YAML incorrectly later.
// Parsed as generic maps rather than Kueue's own Go types, to avoid
// pulling in the whole sigs.k8s.io/kueue module just for a config
// sanity check.
func loadTestdataDocs(t *testing.T) []map[string]interface{} {
	t.Helper()
	raw, err := os.ReadFile("testdata/cluster-queue.yaml")
	if err != nil {
		t.Fatalf("reading testdata: %v", err)
	}
	var docs []map[string]interface{}
	start := 0
	for i := 0; i < len(raw)-4; i++ {
		if raw[i] == '\n' && string(raw[i+1:i+4]) == "---" {
			docs = append(docs, mustParseDoc(t, raw[start:i]))
			start = i + 4
		}
	}
	docs = append(docs, mustParseDoc(t, raw[start:]))
	if len(docs) == 0 {
		t.Fatal("expected at least one YAML document in testdata")
	}
	return docs
}

func mustParseDoc(t *testing.T, doc []byte) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := yaml.Unmarshal(doc, &m); err != nil {
		t.Fatalf("parsing YAML doc: %v", err)
	}
	return m
}

func findByKindAndName(t *testing.T, kind, name string) map[string]interface{} {
	t.Helper()
	for _, doc := range loadTestdataDocs(t) {
		if doc["kind"] != kind {
			continue
		}
		meta, _ := doc["metadata"].(map[string]interface{})
		if meta != nil && meta["name"] == name {
			return doc
		}
	}
	t.Fatalf("%s %q not found in testdata", kind, name)
	return nil
}

func specOf(t *testing.T, doc map[string]interface{}) map[string]interface{} {
	t.Helper()
	spec, ok := doc["spec"].(map[string]interface{})
	if !ok {
		t.Fatal("expected spec to be a map")
	}
	return spec
}

func TestCohortConfig_BothQueuesShareCohort(t *testing.T) {
	finance := specOf(t, findByKindAndName(t, "ClusterQueue", "finance-cluster-queue"))
	research := specOf(t, findByKindAndName(t, "ClusterQueue", "research-cluster-queue"))

	if finance["cohortName"] != "cnip-cohort" {
		t.Errorf("expected finance-cluster-queue cohortName=cnip-cohort, got %v", finance["cohortName"])
	}
	if research["cohortName"] != "cnip-cohort" {
		t.Errorf("expected research-cluster-queue cohortName=cnip-cohort, got %v", research["cohortName"])
	}
	if finance["cohortName"] != research["cohortName"] {
		t.Error("finance and research ClusterQueues must share the same cohortName for cross-tenant fairness to work at all")
	}
}

func TestCohortConfig_ReclaimEnabled(t *testing.T) {
	for _, name := range []string{"finance-cluster-queue", "research-cluster-queue"} {
		spec := specOf(t, findByKindAndName(t, "ClusterQueue", name))
		preemption, ok := spec["preemption"].(map[string]interface{})
		if !ok {
			t.Fatalf("%s: expected preemption to be configured", name)
		}
		if preemption["reclaimWithinCohort"] != "Any" {
			t.Errorf("%s: expected reclaimWithinCohort=Any (required for the fairness proof — a tenant must be able to reclaim its nominal share regardless of the borrower's priority), got %v", name, preemption["reclaimWithinCohort"])
		}
		if preemption["withinClusterQueue"] != "LowerPriority" {
			t.Errorf("%s: expected withinClusterQueue=LowerPriority, got %v", name, preemption["withinClusterQueue"])
		}
	}
}

func TestCohortConfig_BorrowingLimitsMatchNominal(t *testing.T) {
	// Both queues are designed symmetrically: 2 CPU/4Gi nominal, with a
	// borrowing limit equal to nominal (so a tenant can at most double
	// its own share by borrowing the other's fully-idle capacity, never
	// more).
	for _, name := range []string{"finance-cluster-queue", "research-cluster-queue"} {
		spec := specOf(t, findByKindAndName(t, "ClusterQueue", name))
		groups, _ := spec["resourceGroups"].([]interface{})
		if len(groups) != 1 {
			t.Fatalf("%s: expected exactly one resource group", name)
		}
		group, _ := groups[0].(map[string]interface{})
		flavors, _ := group["flavors"].([]interface{})
		if len(flavors) != 1 {
			t.Fatalf("%s: expected exactly one flavor", name)
		}
		flavor, _ := flavors[0].(map[string]interface{})
		resources, _ := flavor["resources"].([]interface{})

		found := map[string]bool{}
		for _, r := range resources {
			res, _ := r.(map[string]interface{})
			resName, _ := res["name"].(string)
			found[resName] = true
			nominal, hasNominal := res["nominalQuota"]
			borrowing, hasBorrowing := res["borrowingLimit"]
			if !hasBorrowing {
				t.Errorf("%s/%s: expected a borrowingLimit to be set", name, resName)
				continue
			}
			if nominal != borrowing {
				t.Errorf("%s/%s: expected borrowingLimit to equal nominalQuota (%v), got %v", name, resName, nominal, borrowing)
			}
			_ = hasNominal
		}
		if !found["cpu"] || !found["memory"] {
			t.Errorf("%s: expected both cpu and memory resources configured", name)
		}
	}
}

func TestCohortConfig_PriorityClassValues(t *testing.T) {
	want := map[string]float64{
		"platform-interactive": 1000,
		"platform-batch":       333,
		"platform-background":  100,
	}
	found := map[string]float64{}
	for _, doc := range loadTestdataDocs(t) {
		if doc["kind"] != "WorkloadPriorityClass" {
			continue
		}
		meta, _ := doc["metadata"].(map[string]interface{})
		name, _ := meta["name"].(string)
		value, ok := doc["value"].(float64)
		if !ok {
			t.Errorf("%s: expected value to be numeric", name)
			continue
		}
		found[name] = value
	}
	for name, wantValue := range want {
		gotValue, ok := found[name]
		if !ok {
			t.Errorf("expected WorkloadPriorityClass %q to exist", name)
			continue
		}
		if gotValue != wantValue {
			t.Errorf("%s: expected value %v, got %v", name, wantValue, gotValue)
		}
	}
}
