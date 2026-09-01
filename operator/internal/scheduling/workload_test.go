package scheduling

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	platformv1alpha1 "github.com/devam1402/cloud-native-inference-platform/operator/api/v1alpha1"
)

func TestCPURequest(t *testing.T) {
	cases := []struct {
		class      string
		wantCPU    string
		wantMemory string
	}{
		{"interactive", "500m", "1Gi"},
		{"batch", "250m", "512Mi"},
		{"background", "100m", "256Mi"},
		{"unknown-class", "250m", "512Mi"}, // falls through to default
	}
	for _, c := range cases {
		cpu, mem := CPURequest(c.class)
		if cpu.String() != c.wantCPU {
			t.Errorf("%s: expected cpu %s, got %s", c.class, c.wantCPU, cpu.String())
		}
		if mem.String() != c.wantMemory {
			t.Errorf("%s: expected memory %s, got %s", c.class, c.wantMemory, mem.String())
		}
	}
}

func TestBuildJob(t *testing.T) {
	isvc := &platformv1alpha1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-isvc",
			Namespace: "finance",
			UID:       types.UID("abc-123"),
		},
		Spec: platformv1alpha1.InferenceServiceSpec{
			WorkloadClass: "interactive",
		},
	}

	job := BuildJob(isvc, "finance-queue")

	if job.Name != "test-isvc" {
		t.Errorf("expected job name test-isvc, got %s", job.Name)
	}
	if job.Namespace != "finance" {
		t.Errorf("expected namespace finance, got %s", job.Namespace)
	}
	if job.Labels[KueueQueueLabel] != "finance-queue" {
		t.Errorf("expected queue label finance-queue, got %s", job.Labels[KueueQueueLabel])
	}
	if job.Spec.Suspend == nil || !*job.Spec.Suspend {
		t.Error("expected job to be created suspended — Kueue must be able to gate admission")
	}
	if len(job.OwnerReferences) != 1 {
		t.Fatalf("expected exactly 1 owner reference, got %d", len(job.OwnerReferences))
	}
	if job.OwnerReferences[0].Kind != "InferenceService" || job.OwnerReferences[0].Name != "test-isvc" {
		t.Errorf("owner reference doesn't correctly point at the InferenceService: %+v", job.OwnerReferences[0])
	}
	if job.OwnerReferences[0].Controller == nil || !*job.OwnerReferences[0].Controller {
		t.Error("expected Controller=true on owner reference")
	}

	container := job.Spec.Template.Spec.Containers[0]
	cpuReq := container.Resources.Requests["cpu"]
	if cpuReq.String() != "500m" {
		t.Errorf("expected interactive-class cpu request 500m, got %s", cpuReq.String())
	}
}
