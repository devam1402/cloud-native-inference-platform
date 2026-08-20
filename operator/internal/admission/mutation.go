package admission

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	platformv1alpha1 "github.com/devam1402/cloud-native-inference-platform/operator/api/v1alpha1"
	"github.com/devam1402/cloud-native-inference-platform/operator/internal/priority"
)

const (
	AnnotationTenant          = "platform.platform.io/tenant"
	AnnotationDerivedPriority = "platform.platform.io/derived-priority"
	// AnnotationFairnessID has no consumer yet — llm-d/EPP integration
	// isn't built. Injecting it now is forward-looking and cheap; it
	// means that integration is a smaller diff later. Currently set to
	// the tenant name, matching the same-tenant-traffic grouping already
	// used by NetworkPolicy.
	AnnotationFairnessID = "platform.platform.io/fairness-id"
)

// MutateInferenceService injects tenant identity and derived priority as
// annotations. Idempotent by construction: setting the same annotation
// value twice on re-admission (an update, or a retry) is a no-op map
// write, not an error — unlike the reconcile-loop change-detection
// pattern used elsewhere, no explicit "already correct" check is needed
// here since annotation assignment has no side effect to avoid repeating.
func MutateInferenceService(ctx context.Context, c client.Client, isvc *platformv1alpha1.InferenceService) error {
	if isvc.Annotations == nil {
		isvc.Annotations = map[string]string{}
	}

	isvc.Annotations[AnnotationTenant] = isvc.Namespace
	isvc.Annotations[AnnotationFairnessID] = isvc.Namespace

	var tenant platformv1alpha1.Tenant
	err := c.Get(ctx, types.NamespacedName{Name: isvc.Namespace}, &tenant)
	if apierrors.IsNotFound(err) {
		// tenant doesn't exist — validation should have already caught
		// this and denied the request, but mutation runs independently;
		// don't set a priority annotation we can't derive correctly
		return nil
	}
	if err != nil {
		return err
	}

	derived := priority.Calculate(tenant.Spec.Priority.Default, isvc.Spec.WorkloadClass, isvc.Spec.Priority)
	isvc.Annotations[AnnotationDerivedPriority] = itoa(derived)
	return nil
}

func itoa(i int32) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [12]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
