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
			t.Errorf(
				"%s: expected cpu %s, got %s",
				c.class,
				c.wantCPU,
				cpu.String(),
			)
		}

		if mem.String() != c.wantMemory {
			t.Errorf(
				"%s: expected memory %s, got %s",
				c.class,
				c.wantMemory,
				mem.String(),
			)
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
		t.Errorf(
			"expected queue label finance-queue, got %s",
			job.Labels[KueueQueueLabel],
		)
	}

	if job.Spec.Suspend == nil || !*job.Spec.Suspend {
		t.Error(
			"expected job to be created suspended — Kueue must be able to gate admission",
		)
	}

	if len(job.OwnerReferences) != 1 {
		t.Fatalf(
			"expected exactly 1 owner reference, got %d",
			len(job.OwnerReferences),
		)
	}

	if job.OwnerReferences[0].Kind != "InferenceService" ||
		job.OwnerReferences[0].Name != "test-isvc" {
		t.Errorf(
			"owner reference doesn't correctly point at the InferenceService: %+v",
			job.OwnerReferences[0],
		)
	}

	if job.OwnerReferences[0].Controller == nil ||
		!*job.OwnerReferences[0].Controller {
		t.Error("expected Controller=true on owner reference")
	}

	container := job.Spec.Template.Spec.Containers[0]

	cpuReq := container.Resources.Requests["cpu"]
	if cpuReq.String() != "500m" {
		t.Errorf(
			"expected interactive-class cpu request 500m, got %s",
			cpuReq.String(),
		)
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
			t.Errorf(
				"%s: expected %s, got %s",
				class,
				want,
				got,
			)
		}
	}
}

func TestBuildJob_PriorityClassLabel(t *testing.T) {
	isvc := &platformv1alpha1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "prio-test",
			Namespace: "finance",
			UID:       types.UID("xyz"),
		},
		Spec: platformv1alpha1.InferenceServiceSpec{
			WorkloadClass: "background",
		},
	}

	job := BuildJob(isvc, "finance-queue")

	if job.Labels[KueuePriorityClassLabel] != "platform-background" {
		t.Errorf(
			"expected priority class label platform-background, got %s",
			job.Labels[KueuePriorityClassLabel],
		)
	}
}

func TestBuildJob_SatisfiesRestrictedPodSecurity(t *testing.T) {
	isvc := &platformv1alpha1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sec-test",
			Namespace: "finance",
			UID:       types.UID("sec-1"),
		},
		Spec: platformv1alpha1.InferenceServiceSpec{
			WorkloadClass: "interactive",
		},
	}

	job := BuildJob(isvc, "finance-queue")

	podSpec := job.Spec.Template.Spec

	if podSpec.SecurityContext == nil ||
		podSpec.SecurityContext.SeccompProfile == nil {
		t.Fatal(
			"expected pod-level seccompProfile to satisfy restricted Pod Security Standard",
		)
	}

	container := podSpec.Containers[0]
	sc := container.SecurityContext

	if sc == nil {
		t.Fatal("expected container securityContext to be set")
	}

	if sc.AllowPrivilegeEscalation == nil ||
		*sc.AllowPrivilegeEscalation {
		t.Error("expected allowPrivilegeEscalation=false")
	}

	if sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
		t.Error("expected runAsNonRoot=true")
	}

	if sc.Capabilities == nil ||
		len(sc.Capabilities.Drop) != 1 ||
		sc.Capabilities.Drop[0] != "ALL" {
		t.Error("expected capabilities.drop=[ALL]")
	}

	if sc.SeccompProfile == nil ||
		sc.SeccompProfile.Type != "RuntimeDefault" {
		t.Error("expected container seccompProfile.type=RuntimeDefault")
	}

	if sc.RunAsUser == nil || *sc.RunAsUser == 0 {
		t.Error(
			"expected non-zero runAsUser — busybox defaults to root and RunAsNonRoot alone is not enough",
		)
	}
}

// TestBuildJob_GPURequest verifies that the existing GPU behavior
// still requests one full NVIDIA GPU when GPUType is unset.
func TestBuildJob_GPURequest(t *testing.T) {
	gpuTrue := true

	isvc := &platformv1alpha1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gpu-test",
			Namespace: "finance",
			UID:       types.UID("gpu-1"),
		},
		Spec: platformv1alpha1.InferenceServiceSpec{
			WorkloadClass: "interactive",
			GPU:           &gpuTrue,
		},
	}

	job := BuildJob(isvc, "finance-gpu-queue")

	container := job.Spec.Template.Spec.Containers[0]

	gpuLimit := container.Resources.Limits["nvidia.com/gpu"]

	if gpuLimit.String() != "1" {
		t.Errorf(
			"expected nvidia.com/gpu limit of 1, got %s",
			gpuLimit.String(),
		)
	}

	if container.Image != "nvidia/cuda:12.4.0-base-ubuntu22.04" {
		t.Errorf(
			"expected CUDA image, got %s",
			container.Image,
		)
	}

	if job.Spec.Template.Spec.RuntimeClassName == nil ||
		*job.Spec.Template.Spec.RuntimeClassName != "nvidia" {
		t.Error("expected runtimeClassName=nvidia on the pod spec")
	}

	if job.Labels[KueueQueueLabel] != "finance-gpu-queue" {
		t.Errorf(
			"expected queue label finance-gpu-queue, got %s",
			job.Labels[KueueQueueLabel],
		)
	}
}

// TestBuildJob_MIGRequest verifies that an InferenceService explicitly
// requesting the H100 3g.40gb MIG profile gets one MIG resource and
// does not accidentally request the full GPU.
func TestBuildJob_MIGRequest(t *testing.T) {
	gpuTrue := true

	isvc := &platformv1alpha1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mig-test",
			Namespace: "finance",
			UID:       types.UID("mig-1"),
		},
		Spec: platformv1alpha1.InferenceServiceSpec{
			WorkloadClass: "interactive",
			GPU:           &gpuTrue,
			GPUType:       "mig-3g.40gb",
		},
	}

	job := BuildJob(isvc, "finance-gpu-queue")

	container := job.Spec.Template.Spec.Containers[0]

	// The MIG workload must request exactly one MIG slice.
	migLimit := container.Resources.Limits["nvidia.com/mig-3g.40gb"]

	if migLimit.String() != "1" {
		t.Errorf(
			"expected nvidia.com/mig-3g.40gb limit of 1, got %s",
			migLimit.String(),
		)
	}

	// The MIG workload must not request a full GPU.
	if _, hasFullGPU := container.Resources.Limits["nvidia.com/gpu"]; hasFullGPU {
		t.Error(
			"expected no nvidia.com/gpu limit for MIG workload",
		)
	}

	// MIG workloads still use the NVIDIA runtime.
	if job.Spec.Template.Spec.RuntimeClassName == nil ||
		*job.Spec.Template.Spec.RuntimeClassName != "nvidia" {
		t.Error(
			"expected runtimeClassName=nvidia on MIG pod spec",
		)
	}

	// Kueue queue handling must remain unchanged.
	if job.Labels[KueueQueueLabel] != "finance-gpu-queue" {
		t.Errorf(
			"expected queue label finance-gpu-queue, got %s",
			job.Labels[KueueQueueLabel],
		)
	}
}

// TestBuildJob_NonGPUUnaffectedByGPUField verifies that explicitly
// setting GPU=false keeps the workload on the CPU-only path.
func TestBuildJob_NonGPUUnaffectedByGPUField(t *testing.T) {
	gpuFalse := false

	isvc := &platformv1alpha1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cpu-test",
			Namespace: "finance",
			UID:       types.UID("cpu-1"),
		},
		Spec: platformv1alpha1.InferenceServiceSpec{
			WorkloadClass: "interactive",
			GPU:           &gpuFalse,
		},
	}

	job := BuildJob(isvc, "finance-queue")

	container := job.Spec.Template.Spec.Containers[0]

	if container.Image != "busybox:1.36" {
		t.Errorf(
			"expected busybox image when GPU is false, got %s",
			container.Image,
		)
	}

	if _, hasGPU := container.Resources.Limits["nvidia.com/gpu"]; hasGPU {
		t.Error(
			"expected no nvidia.com/gpu limit when GPU is false",
		)
	}

	if job.Spec.Template.Spec.RuntimeClassName != nil {
		t.Error(
			"expected no runtimeClassName set for non-GPU workloads",
		)
	}
}
