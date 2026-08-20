package admission

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	platformv1alpha1 "github.com/devam1402/cloud-native-inference-platform/operator/api/v1alpha1"
	"github.com/devam1402/cloud-native-inference-platform/operator/internal/resourceprofile"
)

// ValidateInferenceService runs the checks that matter to catch at
// admission time — cheap, structural checks first, then anything
// needing a client read. Checks that only make sense post-hoc (e.g.
// "is the Model actually verified yet") stay in InferenceServiceController,
// per the "don't put all logic in the webhook" principle — admission
// enforces what's knowable and wrong right now, the controller handles
// what resolves over time.
func ValidateInferenceService(ctx context.Context, c client.Client, isvc *platformv1alpha1.InferenceService) *AdmissionDecision {
	if d := validateTenantExists(ctx, c, isvc); !d.Allowed {
		return d
	}
	if d := validateResourceProfile(isvc); !d.Allowed {
		return d
	}
	if d := validateSLO(isvc); !d.Allowed {
		return d
	}
	if d := validateSignedModelPolicy(ctx, c, isvc); !d.Allowed {
		return d
	}
	return Allow()
}

func validateTenantExists(ctx context.Context, c client.Client, isvc *platformv1alpha1.InferenceService) *AdmissionDecision {
	var tenant platformv1alpha1.Tenant
	err := c.Get(ctx, types.NamespacedName{Name: isvc.Namespace}, &tenant)
	if apierrors.IsNotFound(err) {
		return Deny(
			"TENANT_NOT_FOUND",
			fmt.Sprintf("tenant %q does not exist", isvc.Namespace),
			"create the Tenant before submitting InferenceService objects in its namespace",
		)
	}
	if err != nil {
		// a real API error, not "doesn't exist" — fail closed rather
		// than silently allowing through on an ambiguous error
		return Deny("TENANT_LOOKUP_ERROR", err.Error(), "retry; if this persists, check API server health")
	}
	return Allow()
}

func validateResourceProfile(isvc *platformv1alpha1.InferenceService) *AdmissionDecision {
	if _, err := resourceprofile.Resolve(isvc.Spec.ResourceProfile); err != nil {
		return Deny(
			"PROFILE_INVALID",
			err.Error(),
			"use one of the known profiles: protected-gpu, shared-gpu, batch-shared",
		)
	}
	return Allow()
}

func validateSLO(isvc *platformv1alpha1.InferenceService) *AdmissionDecision {
	slo := isvc.Spec.SLO
	if slo.TTFTP99Milliseconds != nil && *slo.TTFTP99Milliseconds <= 0 {
		return Deny("SLO_INVALID", "slo.ttftP99 must be positive", "set a realistic millisecond value, or omit the field")
	}
	if slo.TPOTP99Milliseconds != nil && *slo.TPOTP99Milliseconds <= 0 {
		return Deny("SLO_INVALID", "slo.tpotP99 must be positive", "set a realistic millisecond value, or omit the field")
	}
	return Allow()
}

// validateSignedModelPolicy enforces PlatformPolicy.Spec.SignedModels.Required
// structurally at admission time: if the platform requires signed models,
// the InferenceService's referenced Model must be spec'd to require a
// signed image. This doesn't check whether the Model is actually verified
// yet — that's a readiness concern for the controller, not an admission
// concern (a Model can legitimately not exist yet if it's about to be
// created in the same apply).
func validateSignedModelPolicy(ctx context.Context, c client.Client, isvc *platformv1alpha1.InferenceService) *AdmissionDecision {
	policy, err := getEffectivePlatformPolicy(ctx, c)
	if err != nil || policy == nil || !policy.Spec.SignedModels.Required {
		return Allow()
	}

	var model platformv1alpha1.Model
	err = c.Get(ctx, types.NamespacedName{Name: isvc.Spec.Model, Namespace: isvc.Namespace}, &model)
	if apierrors.IsNotFound(err) {
		// model doesn't exist yet — nothing to check structurally; the
		// controller will keep InferenceService Pending until it does
		return Allow()
	}
	if err != nil {
		return Deny("MODEL_LOOKUP_ERROR", err.Error(), "retry; if this persists, check API server health")
	}

	if !model.Spec.RequireSignedImage {
		return Deny(
			"SIGNED_MODEL_REQUIRED",
			fmt.Sprintf("platform policy requires signed models, but Model %q does not set requireSignedImage", isvc.Spec.Model),
			"set spec.requireSignedImage: true on the Model",
		)
	}
	return Allow()
}

// getEffectivePlatformPolicy fetches the single cluster-wide PlatformPolicy
// named "default". Returns (nil, nil) if none exists — absence of a
// PlatformPolicy means no additional constraints, not a validation error.
func getEffectivePlatformPolicy(ctx context.Context, c client.Client) (*platformv1alpha1.PlatformPolicy, error) {
	var policy platformv1alpha1.PlatformPolicy
	err := c.Get(ctx, types.NamespacedName{Name: "default"}, &policy)
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &policy, nil
}
