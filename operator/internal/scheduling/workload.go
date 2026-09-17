package scheduling

import (
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	platformv1alpha1 "github.com/devam1402/cloud-native-inference-platform/operator/api/v1alpha1"
)

// KueueQueueLabel is the label Kueue's job integration watches to know
// which LocalQueue a Job belongs to.
const KueueQueueLabel = "kueue.x-k8s.io/queue-name"

// KueuePriorityClassLabel is the label Kueue reads to determine the
// WorkloadPriorityClass.
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

// ReplicaCount returns the requested replica count,
// defaulting to 1 when Replicas is unset or invalid.
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
func WantsGPU(isvc *platformv1alpha1.InferenceService) bool {
	return isvc.Spec.GPU != nil && *isvc.Spec.GPU
}

// GPUResourceName returns the Kubernetes resource that should be requested
// for this InferenceService.
//
// GPUType:
//   - ""             -> nvidia.com/gpu
//   - "full"         -> nvidia.com/gpu
//   - "mig-3g.40gb"  -> nvidia.com/mig-3g.40gb
//
// The empty/default case intentionally preserves the platform's existing
// full-GPU behavior.
func GPUResourceName(isvc *platformv1alpha1.InferenceService) corev1.ResourceName {
	switch isvc.Spec.GPUType {
	case "mig-3g.40gb":
		return corev1.ResourceName("nvidia.com/mig-3g.40gb")

	case "full", "":
		fallthrough

	default:
		return corev1.ResourceName("nvidia.com/gpu")
	}
}

// restrictedSecurityContext satisfies the "restricted" Pod Security
// Standard for CPU-only workloads.
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

// gpuSecurityContext is used for NVIDIA GPU workloads.
//
// The NVIDIA runtime class provides access to the GPU device. We deliberately
// do not apply the CPU-only restricted security context here because the CUDA
// image used by the current placeholder workload is not built around the same
// non-root assumptions.
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
//
// GPU behavior:
//
//	GPU: true, GPUType omitted
//	    -> nvidia.com/gpu: 1
//
//	GPU: true, GPUType: full
//	    -> nvidia.com/gpu: 1
//
//	GPU: true, GPUType: mig-3g.40gb
//	    -> nvidia.com/mig-3g.40gb: 1
//
// Kubernetes/Kueue/device-plugin/scheduler are responsible for deciding
// which physical GPU or MIG device satisfies the resource request.
//
// The operator does NOT assign a specific MIG UUID such as:
//
//	MIG-7bf8cfac-...
//
// That is intentionally left to Kubernetes.
func ModelURI(model *platformv1alpha1.Model) string {
	if model == nil {
		return ""
	}

	return model.Spec.Source.URI
}

func BuildJob(
	isvc *platformv1alpha1.InferenceService,
	model *platformv1alpha1.Model,
	localQueueName string,
) *batchv1.Job {
	priorityClass := PriorityClassForWorkloadClass(isvc.Spec.WorkloadClass)
	replicas := ReplicaCount(isvc)
	wantsGPU := WantsGPU(isvc)

	backoffLimit := int32(0)
	suspend := true

	var container corev1.Container

	if wantsGPU {
		// Select the Kubernetes GPU resource based on GPUType.
		//
		// Default behavior remains:
		//     nvidia.com/gpu: 1
		//
		// MIG behavior:
		//     nvidia.com/mig-3g.40gb: 1
		gpuResourceName := GPUResourceName(isvc)

		container = corev1.Container{
			Name:  "vllm",
			Image: "vllm/vllm-openai:v0.29.0",
			Args: []string{
				"--model",
				ModelURI(model),
				"--host",
				"0.0.0.0",
				"--port",
				"8000",
			},

			SecurityContext: gpuSecurityContext(),

			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					gpuResourceName: resource.MustParse("1"),
				},
			},
		}
	} else {
		cpu, memory := CPURequest(isvc.Spec.WorkloadClass)

		container = corev1.Container{
			// Placeholder workload.
			//
			// A real serving container such as vLLM belongs to the
			// inference-serving implementation and is not introduced
			// here merely to add MIG scheduling.
			Name:    "placeholder",
			Image:   "busybox:1.36",
			Command: []string{"sleep", "30"},

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
		// CPU-only workloads use the restricted Pod Security configuration.
		podSpec.SecurityContext = &corev1.PodSecurityContext{
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		}
	} else {
		// GPU workloads use the NVIDIA runtime class.
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

func boolPtr(b bool) *bool {
	return &b
}

func int64Ptr(i int64) *int64 {
	return &i
}

func int32Ptr(i int32) *int32 {
	return &i
}

func stringPtr(s string) *string {
	return &s
}
