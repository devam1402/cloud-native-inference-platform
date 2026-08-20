// Package webhook wires internal/admission's pure validation and
// mutation logic into controller-runtime's admission handler interfaces.
// No business logic lives here — this is glue only, so a bug in the
// webhook framework wiring is easy to distinguish from a bug in the
// actual validation/mutation rules (which are unit-tested independently
// in internal/admission).
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

// InferenceServiceValidator implements admission.CustomValidator.
type InferenceServiceValidator struct {
	Client client.Client
}

var _ admission.CustomValidator = &InferenceServiceValidator{}

func (v *InferenceServiceValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	isvc, ok := obj.(*platformv1alpha1.InferenceService)
	if !ok {
		return nil, fmt.Errorf("expected an InferenceService, got %T", obj)
	}
	decision := internaladmission.ValidateInferenceService(ctx, v.Client, isvc)
	if !decision.Allowed {
		return nil, decision
	}
	return nil, nil
}

func (v *InferenceServiceValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	isvc, ok := newObj.(*platformv1alpha1.InferenceService)
	if !ok {
		return nil, fmt.Errorf("expected an InferenceService, got %T", newObj)
	}
	decision := internaladmission.ValidateInferenceService(ctx, v.Client, isvc)
	if !decision.Allowed {
		return nil, decision
	}
	return nil, nil
}

// ValidateDelete allows all deletes — nothing in the validation checklist
// applies to removing an object, and blocking deletes is a separate,
// deliberate policy decision this platform hasn't made.
func (v *InferenceServiceValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// SetupInferenceServiceWebhook registers this validator with the manager.
// Not called from anywhere yet — cmd/main.go wiring happens once
// Certificate/Issuer manifests exist and the webhook is ready to
// actually register against the live API server.
func SetupInferenceServiceWebhook(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&platformv1alpha1.InferenceService{}).
		WithValidator(&InferenceServiceValidator{Client: mgr.GetClient()}).
		Complete()
}
