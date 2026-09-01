// Package scheduling translates InferenceService objects into Kueue-
// admissible Kubernetes workloads (batch/v1 Jobs labeled for Kueue's
// job integration). Resource sizing here is deliberately simple —
// CPU/memory requests keyed by workload class — since this is CPU-only
// scheduling (P3); GPU-aware sizing is P3.5's job, gated on deciding how
// a rented GPU node joins the cluster.
package scheduling

import "k8s.io/apimachinery/pkg/api/resource"

// CPURequest returns default CPU/memory requests for a workload class.
// Not derived from ResourceProfile — that package models GPU
// characteristics (memory, sharing, preemptibility), not CPU sizing,
// and conflating the two would overload its meaning.
func CPURequest(workloadClass string) (cpu, memory resource.Quantity) {
	switch workloadClass {
	case "interactive":
		return resource.MustParse("500m"), resource.MustParse("1Gi")
	case "batch":
		return resource.MustParse("250m"), resource.MustParse("512Mi")
	case "background":
		return resource.MustParse("100m"), resource.MustParse("256Mi")
	default:
		return resource.MustParse("250m"), resource.MustParse("512Mi")
	}
}
