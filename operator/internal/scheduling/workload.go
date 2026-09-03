package scheduling

import (
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
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
// small set of named tiers. PriorityClassForWorkloadClass below maps
// the platform's existing three-tier workloadClass system onto three
// WorkloadPriorityClass objects (platform-interactive/batch/background,
// created once on the cluster) rather than inventing a second scheme.
const KueuePriorityClassLabel = "kueue.x-k8s.io/priority-class"

// PriorityClassForWorkloadClass returns the WorkloadPriorityClass name
// for a given InferenceService workload class. Falls through to
// platform-batch for an unrecognized class, matching CPURequest's
// same fallback behavior.
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

// restrictedSecurityContext satisfies the "restricted" Pod Security
// Standard, which every tenant namespace enforces (TenantController sets
// pod-security.kubernetes.io/enforce=restricted). Without this, every
// Pod the Job controller tries to create is silently rejected and
// retried forever — the Job and its Kueue Workload both look healthy
// (Running/Admitted) even though zero Pods ever actually start. That
// bug was live undetected through the whole priority/preemption/fairness
// proof sequence, since Kueue reserves quota at Workload-admission time,
// independent of whether the underlying Pod can start — the scheduling
// layer was genuinely correct throughout; only Pod execution was broken.
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

// BuildJob renders a Kueue-admissible batch/v1 Job for an InferenceService.
// The Job's name matches the InferenceService's name so
// TenantController-style CreateOrUpdate reconciliation stays idempotent —
// re-running this on an unchanged InferenceService produces the same Job.
//
// localQueueName is the tenant's LocalQueue (by convention,
// "<tenant>-queue", matching the finance-queue/research-queue pattern
// already created manually — a future TenantController change could
// create this automatically per Tenant, same as NetworkPolicy/RBAC).
func BuildJob(isvc *platformv1alpha1.InferenceService, localQueueName string) *batchv1.Job {
	cpu, memory := CPURequest(isvc.Spec.WorkloadClass)
	priorityClass := PriorityClassForWorkloadClass(isvc.Spec.WorkloadClass)

	backoffLimit := int32(0) // don't retry — a failed InferenceService job should surface as failed, not silently retry
	suspend := true          // Kueue unsuspends this once admitted; never start unsuspended

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
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"platform.platform.io/inferenceservice": isvc.Name,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					SecurityContext: &corev1.PodSecurityContext{
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{
						{
							// Placeholder workload — a real serving container
							// (vLLM, TGI, etc.) is P3.5/inference-serving scope.
							// This proves scheduling semantics, not serving.
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
						},
					},
				},
			},
		},
	}
}

func boolPtr(b bool) *bool    { return &b }
func int64Ptr(i int64) *int64 { return &i }
