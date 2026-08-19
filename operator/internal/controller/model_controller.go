package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	platformv1alpha1 "github.com/devam1402/cloud-native-inference-platform/operator/api/v1alpha1"
)

type ModelReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=platform.platform.io,resources=models,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=platform.platform.io,resources=models/status,verbs=get;update;patch

func (r *ModelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var model platformv1alpha1.Model
	if err := r.Get(ctx, req.NamespacedName, &model); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// verifyReason is "" when verification succeeds, or a human-readable
	// reason string when it fails. This is a status outcome, not a Go
	// error — an unsigned model is an expected, valid Pending state, not
	// a reconcile failure that should trigger controller-runtime retries.
	verifyReason := verifyArtifact(&model)

	if _, err := r.updateStatus(ctx, &model, verifyReason); err != nil {
		return ctrl.Result{}, err
	}

	if verifyReason != "" {
		log.V(1).Info("model not yet verified", "model", model.Name, "reason", verifyReason)
	}
	return ctrl.Result{}, nil
}

// verifyArtifact returns "" if the model verifies successfully, or a
// human-readable failure reason otherwise. It never returns a Go error —
// callers should distinguish "not ready yet" from "the API call itself
// failed" at a different layer.
func verifyArtifact(m *platformv1alpha1.Model) string {
	if m.Spec.Source.URI == "" {
		return "model source URI is empty"
	}
	if m.Spec.RequireSignedImage && m.Spec.Source.Digest == "" {
		return "requireSignedImage is true but no digest provided"
	}
	m.Status.ResolvedDigest = m.Spec.Source.Digest
	return ""
}

func (r *ModelReconciler) updateStatus(ctx context.Context, m *platformv1alpha1.Model, verifyReason string) (bool, error) {
	status := metav1.ConditionTrue
	reason := "ArtifactVerified"
	msg := ""
	if verifyReason != "" {
		status = metav1.ConditionFalse
		reason = "VerificationFailed"
		msg = verifyReason
	}

	newConditions := append([]metav1.Condition{}, m.Status.Conditions...)
	meta.SetStatusCondition(&newConditions, metav1.Condition{
		Type: "Ready", Status: status, Reason: reason, Message: msg,
	})

	newPhase := "Pending"
	if verifyReason == "" {
		newPhase = "Ready"
	}

	if statusUnchanged(m.Status.Conditions, newConditions) && m.Status.Phase == newPhase {
		return false, nil
	}

	m.Status.Conditions = newConditions
	m.Status.Phase = newPhase
	return true, r.Status().Update(ctx, m)
}

func (r *ModelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1alpha1.Model{}).
		Named("model").
		Complete(r)
}
