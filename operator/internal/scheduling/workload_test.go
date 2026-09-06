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

func TestPriorityClassForWorkloadClass(t *testing.T) {
	cases := map[string]string{
		"interactive":   "platform-interactive",
		"batch":         "platform-batch",
		"background":    "platform-background",
		"unknown-class": "platform-batch",
	}
	for class, want := range cases {
		got := PriorityClassForWorkloadClass(class)
		if got != want {
			t.Errorf("%s: expected %s, got %s", class, want, got)
		}
	}
}

func TestBuildJob_PriorityClassLabel(t *testing.T) {
	isvc := &platformv1alpha1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "prio-test", Namespace: "finance", UID: types.UID("xyz")},
		Spec:       platformv1alpha1.InferenceServiceSpec{WorkloadClass: "background"},
	}
	job := BuildJob(isvc, "finance-queue")
	if job.Labels[KueuePriorityClassLabel] != "platform-background" {
		t.Errorf("expected priority class label platform-background, got %s", job.Labels[KueuePriorityClassLabel])
	}
}

func TestBuildJob_SatisfiesRestrictedPodSecurity(t *testing.T) {
	isvc := &platformv1alpha1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "sec-test", Namespace: "finance", UID: types.UID("sec-1")},
		Spec:       platformv1alpha1.InferenceServiceSpec{WorkloadClass: "interactive"},
	}
	job := BuildJob(isvc, "finance-queue")

	podSpec := job.Spec.Template.Spec
	if podSpec.SecurityContext == nil || podSpec.SecurityContext.SeccompProfile == nil {
		t.Fatal("expected pod-level seccompProfile to satisfy restricted Pod Security Standard")
	}

	container := podSpec.Containers[0]
	sc := container.SecurityContext
	if sc == nil {
		t.Fatal("expected container securityContext to be set")
	}
	if sc.AllowPrivilegeEscalation == nil || *sc.AllowPrivilegeEscalation {
		t.Error("expected allowPrivilegeEscalation=false")
	}
	if sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
		t.Error("expected runAsNonRoot=true")
	}
	if sc.Capabilities == nil || len(sc.Capabilities.Drop) != 1 || sc.Capabilities.Drop[0] != "ALL" {
		t.Error("expected capabilities.drop=[ALL]")
	}
	if sc.SeccompProfile == nil || sc.SeccompProfile.Type != "RuntimeDefault" {
		t.Error("expected container seccompProfile.type=RuntimeDefault")
	}
	if sc.RunAsUser == nil || *sc.RunAsUser == 0 {
		t.Error("expected non-zero runAsUser — busybox defaults to root and RunAsNonRoot alone is not enough")
	}
}

func TestReplicaCount(t *testing.T) {
	one := int32(1)
	four := int32(4)
	zero := int32(0)
	negative := int32(-3)

	cases := []struct {
		name     string
		replicas *int32
		want     int32
	}{
		{"unset defaults to 1", nil, 1},
		{"explicit 1", &one, 1},
		{"explicit 4", &four, 4},
		{"zero falls back to 1", &zero, 1},
		{"negative falls back to 1", &negative, 1},
	}
	for _, c := range cases {
		isvc := &platformv1alpha1.InferenceService{
			Spec: platformv1alpha1.InferenceServiceSpec{Replicas: c.replicas},
		}
		got := ReplicaCount(isvc)
		if got != c.want {
			t.Errorf("%s: expected %d, got %d", c.name, c.want, got)
		}
	}
}

func TestBuildJob_GangScheduling(t *testing.T) {
	four := int32(4)
	isvc := &platformv1alpha1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "gang-test", Namespace: "finance", UID: types.UID("gang-1")},
		Spec: platformv1alpha1.InferenceServiceSpec{
			WorkloadClass: "batch",
			Replicas:      &four,
		},
	}
	job := BuildJob(isvc, "finance-queue")

	if job.Spec.Completions == nil || *job.Spec.Completions != 4 {
		t.Errorf("expected completions=4, got %v", job.Spec.Completions)
	}
	if job.Spec.Parallelism == nil || *job.Spec.Parallelism != 4 {
		t.Errorf("expected parallelism=4, got %v", job.Spec.Parallelism)
	}
}

func TestBuildJob_DefaultsToSinglePod(t *testing.T) {
	isvc := &platformv1alpha1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "single-test", Namespace: "finance", UID: types.UID("single-1")},
		Spec:       platformv1alpha1.InferenceServiceSpec{WorkloadClass: "interactive"},
	}
	job := BuildJob(isvc, "finance-queue")

	if job.Spec.Completions == nil || *job.Spec.Completions != 1 {
		t.Errorf("expected completions=1 by default, got %v", job.Spec.Completions)
	}
	if job.Spec.Parallelism == nil || *job.Spec.Parallelism != 1 {
		t.Errorf("expected parallelism=1 by default, got %v", job.Spec.Parallelism)
	}
}
