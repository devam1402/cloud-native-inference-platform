package scheduling

import (
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	platformv1alpha1 "github.com/devam1402/cloud-native-inference-platform/operator/api/v1alpha1"
)

// KueueQueueLabel is the label Kueue's job integration watches to know
// which LocalQueue a Job belongs to. Kueue suspends any Job carrying
// this label until its own admission webhook clears it for scheduling —
// suspend:true here is required, not optional, or Kueue never gets a
// chance to gate it.
const KueueQueueLabel = "kueue.x-k8s.io/queue-name"

// KueuePriorityClassLabel is the label Kueue reads to determine a Job's
// WorkloadPriorityClass — a fixed, named object with a numeric value,
// not a raw per-object number. This is a deliberate design choice:
// priority.Calculate() produces a continuous integer (0-1000+) suited to
// human-readable API status, but Kueue's ordering mechanism expects a
// small set of named tiers.
const KueuePriorityClassLabel = "kueue.x-k8s.io/priority-class"

func PriorityClassForWorkloadClass(workloadClass string) string {
	switch workloadClass {
	case "interactive":
		return "platform-interactive"
	case "batch":
		return "platform-batch"
	case "background":
		return "platform-background"
	default:
		return "platform-batch"
	}
}

// ReplicaCount returns the InferenceService's requested replica count,
// defaulting to 1 when Replicas is unset.
func ReplicaCount(isvc *platformv1alpha1.InferenceService) int32 {
	if isvc.Spec.Replicas == nil {
		return 1
	}
	if *isvc.Spec.Replicas < 1 {
		return 1
	}
	return *isvc.Spec.Replicas
}

// WantsGPU reports whether this InferenceService requested a GPU.
// Only a boolean today — this platform has exactly one GPU available
// via MultiKueue dispatch to an external worker cluster; a count field
// would overclaim capability that doesn't yet exist.
func WantsGPU(isvc *platformv1alpha1.InferenceService) bool {
	return isvc.Spec.GPU != nil && *isvc.Spec.GPU
}

// restrictedSecurityContext satisfies the "restricted" Pod Security
// Standard for the CPU-only placeholder image. This does NOT apply to
// GPU workloads — see gpuSecurityContext below.
func restrictedSecurityContext() *corev1.SecurityContext {
	falseVal := false
	trueVal := true
	return &corev1.SecurityContext{
		AllowPrivilegeEscalation: &falseVal,
		RunAsNonRoot:             &trueVal,
		RunAsUser:                int64Ptr(1000),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}

// gpuSecurityContext is deliberately less restrictive than
// restrictedSecurityContext: NVIDIA CUDA base images generally run as
// root and the device-plugin/runtime-class-mediated GPU access path
// doesn't fit the same non-root assumptions the busybox placeholder
// uses. The GPU worker cluster (k3s) does not enforce the "restricted"
// Pod Security Standard the way cnip-gke's tenant namespaces do, so
// this doesn't need to satisfy that policy — it only needs to actually
// let the container start and reach the GPU device.
func gpuSecurityContext() *corev1.SecurityContext {
	falseVal := false
	return &corev1.SecurityContext{
		AllowPrivilegeEscalation: &falseVal,
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
	}
}

// BuildJob renders a Kueue-admissible batch/v1 Job for an InferenceService.
// The Job's name matches the InferenceService's name so
// TenantController-style CreateOrUpdate reconciliation stays idempotent.
//
// GPU workloads: when isvc.Spec.GPU is true, the container requests
// nvidia.com/gpu:1 and uses a CUDA-capable image running nvidia-smi as
// a placeholder proof (same "prove scheduling semantics, not serving"
// scope as the CPU busybox placeholder — a real serving container is
// P3.5/inference-serving scope). The caller (reconcileKueueJob) is
// responsible for choosing the GPU-specific LocalQueue name; BuildJob
// only renders whatever queue name it's given.
//
// Gang scheduling: Completions and Parallelism are both set to the
// replica count, which is what makes Kueue's job integration treat
// this as an atomic gang.
func BuildJob(isvc *platformv1alpha1.InferenceService, localQueueName string) *batchv1.Job {
	priorityClass := PriorityClassForWorkloadClass(isvc.Spec.WorkloadClass)
	replicas := ReplicaCount(isvc)
	wantsGPU := WantsGPU(isvc)

	backoffLimit := int32(0)
	suspend := true

	var container corev1.Container
	if wantsGPU {
		container = corev1.Container{
			Name:            "placeholder",
			Image:           "nvidia/cuda:12.4.0-base-ubuntu22.04",
			Command:         []string{"nvidia-smi"},
			SecurityContext: gpuSecurityContext(),
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("1"),
				},
			},
		}
	} else {
		cpu, memory := CPURequest(isvc.Spec.WorkloadClass)
		container = corev1.Container{
			// Placeholder workload — a real serving container (vLLM,
			// TGI, etc.) is P3.5/inference-serving scope. This proves
			// scheduling semantics, not serving.
			Name:            "placeholder",
			Image:           "busybox:1.36",
			Command:         []string{"sleep", "30"},
			SecurityContext: restrictedSecurityContext(),
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    cpu,
					corev1.ResourceMemory: memory,
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    cpu,
					corev1.ResourceMemory: memory,
				},
			},
		}
	}

	podSpec := corev1.PodSpec{
		RestartPolicy: corev1.RestartPolicyNever,
		Containers:    []corev1.Container{container},
	}
	if !wantsGPU {
		// The restricted PodSecurity standard (enforced on cnip-gke
		// tenant namespaces) requires a pod-level seccompProfile too.
		// The GPU worker cluster doesn't enforce this standard, so it's
		// omitted there rather than fighting compatibility with the
		// NVIDIA runtime class.
		podSpec.SecurityContext = &corev1.PodSecurityContext{
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		}
	} else {
		podSpec.RuntimeClassName = stringPtr("nvidia")
	}

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      isvc.Name,
			Namespace: isvc.Namespace,
			Labels: map[string]string{
				KueueQueueLabel:                         localQueueName,
				KueuePriorityClassLabel:                 priorityClass,
				"platform.platform.io/inferenceservice": isvc.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: platformv1alpha1.GroupVersion.String(),
					Kind:       "InferenceService",
					Name:       isvc.Name,
					UID:        isvc.UID,
					Controller: boolPtr(true),
				},
			},
		},
		Spec: batchv1.JobSpec{
			Suspend:      &suspend,
			BackoffLimit: &backoffLimit,
			Completions:  int32Ptr(replicas),
			Parallelism:  int32Ptr(replicas),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"platform.platform.io/inferenceservice": isvc.Name,
					},
				},
				Spec: podSpec,
			},
		},
	}
}

func boolPtr(b bool) *bool       { return &b }
func int64Ptr(i int64) *int64    { return &i }
func int32Ptr(i int32) *int32    { return &i }
func stringPtr(s string) *string { return &s }
