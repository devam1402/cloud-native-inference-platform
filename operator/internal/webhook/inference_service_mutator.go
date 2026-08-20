package webhook

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	platformv1alpha1 "github.com/devam1402/cloud-native-inference-platform/operator/api/v1alpha1"
	internaladmission "github.com/devam1402/cloud-native-inference-platform/operator/internal/admission"
)

// InferenceServiceMutator implements admission.CustomDefaulter.
type InferenceServiceMutator struct {
	Client client.Client
}

var _ admission.CustomDefaulter = &InferenceServiceMutator{}

// Default is called on both create and update — MutateInferenceService is
// idempotent by construction (see internal/admission/mutation.go), so
// running it again on every update is safe and correct, not wasted work.
func (m *InferenceServiceMutator) Default(ctx context.Context, obj runtime.Object) error {
	isvc, ok := obj.(*platformv1alpha1.InferenceService)
	if !ok {
		return fmt.Errorf("expected an InferenceService, got %T", obj)
	}
	return internaladmission.MutateInferenceService(ctx, m.Client, isvc)
}

// SetupInferenceServiceMutator registers this defaulter with the manager.
// Not called from anywhere yet — same gating as SetupInferenceServiceWebhook.
func SetupInferenceServiceMutator(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&platformv1alpha1.InferenceService{}).
		WithDefaulter(&InferenceServiceMutator{Client: mgr.GetClient()}).
		Complete()
}
