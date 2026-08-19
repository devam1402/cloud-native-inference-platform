package controller

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	platformv1alpha1 "github.com/devam1402/cloud-native-inference-platform/operator/api/v1alpha1"
	"github.com/devam1402/cloud-native-inference-platform/operator/internal/capacityenvelope"
	"github.com/devam1402/cloud-native-inference-platform/operator/internal/priority"
	"github.com/devam1402/cloud-native-inference-platform/operator/internal/resourceprofile"
)

type InferenceServiceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=platform.platform.io,resources=inferenceservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=platform.platform.io,resources=inferenceservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=platform.platform.io,resources=models,verbs=get;list;watch
// +kubebuilder:rbac:groups=platform.platform.io,resources=tenants,verbs=get;list;watch

func (r *InferenceServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var isvc platformv1alpha1.InferenceService
	if err := r.Get(ctx, req.NamespacedName, &isvc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var derivedPriority int32

	var model platformv1alpha1.Model
	var modelState string
	err := r.Get(ctx, types.NamespacedName{Name: isvc.Spec.Model, Namespace: isvc.Namespace}, &model)
	switch {
	case apierrors.IsNotFound(err):
		modelState = "notfound"
	case err != nil:
		return ctrl.Result{}, fmt.Errorf("getting model %q: %w", isvc.Spec.Model, err)
	case meta.IsStatusConditionTrue(model.Status.Conditions, "Ready"):
		modelState = "ready"
	default:
		modelState = "notready"
	}

	var tenant platformv1alpha1.Tenant
	var tenantState string
	err = r.Get(ctx, types.NamespacedName{Name: isvc.Namespace}, &tenant)
	switch {
	case apierrors.IsNotFound(err):
		tenantState = "notfound"
	case err != nil:
		return ctrl.Result{}, fmt.Errorf("getting tenant %q: %w", isvc.Namespace, err)
	default:
		tenantState = "found"
		derivedPriority = priority.Calculate(tenant.Spec.Priority.Default, isvc.Spec.WorkloadClass, isvc.Spec.Priority)
	}

	_, profileErr := resourceprofile.Resolve(isvc.Spec.ResourceProfile)

	var capacityErr error
	if modelState == "ready" && profileErr == nil {
		envelope, cErr := capacityenvelope.Check(isvc.Spec.Model, isvc.Spec.ResourceProfile)
		if cErr != nil {
			capacityErr = cErr
		} else if !envelope.Available {
			capacityErr = fmt.Errorf("no capacity available for model %q on profile %q", isvc.Spec.Model, isvc.Spec.ResourceProfile)
		}
	}

	var modelErr, tenantErr error
	if modelState != "ready" {
		if modelState == "notfound" {
			modelErr = fmt.Errorf("model %q not found", isvc.Spec.Model)
		} else {
			modelErr = fmt.Errorf("model %q is not ready", isvc.Spec.Model)
		}
	}
	if tenantState != "found" {
		tenantErr = fmt.Errorf("tenant %q not found", isvc.Namespace)
	}

	changed, err := r.updateStatus(ctx, &isvc, derivedPriority, modelErr, tenantErr, profileErr, capacityErr)
	if err != nil {
		return ctrl.Result{}, err
	}

	if modelErr != nil || tenantErr != nil || profileErr != nil || capacityErr != nil {
		// The Model watch below normally makes this poll unnecessary once
		// the model becomes ready, but this stays as a safety net for
		// cases the watch doesn't cover (e.g. tenant/profile/capacity
		// changes with no corresponding watch yet).
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	if changed {
		log.Info("inferenceservice reconciled", "inferenceservice", isvc.Name, "priority", derivedPriority)
	}
	return ctrl.Result{}, nil
}

func (r *InferenceServiceReconciler) updateStatus(ctx context.Context, isvc *platformv1alpha1.InferenceService, derivedPriority int32, modelErr, tenantErr, profileErr, capacityErr error) (bool, error) {
	newConditions := append([]metav1.Condition{}, isvc.Status.Conditions...)

	setCond := func(condType string, err error, okReason, failReason string) {
		status := metav1.ConditionTrue
		reason := okReason
		msg := ""
		if err != nil {
			status = metav1.ConditionFalse
			reason = failReason
			msg = err.Error()
		}
		meta.SetStatusCondition(&newConditions, metav1.Condition{
			Type: condType, Status: status, Reason: reason, Message: msg,
		})
	}

	setCond("ModelReady", modelErr, "ModelVerified", "ModelNotReady")
	setCond("TenantResolved", tenantErr, "TenantFound", "TenantNotFound")
	setCond("ResourceProfileResolved", profileErr, "ProfileFound", "ProfileNotFound")
	setCond("CapacityAvailable", capacityErr, "WithinEnvelope", "CapacityUnavailable")

	ready := modelErr == nil && tenantErr == nil && profileErr == nil && capacityErr == nil
	var readyErr error
	if !ready {
		readyErr = fmt.Errorf("not all dependencies satisfied")
	}
	setCond("Accepted", nil, "InferenceServiceObserved", "")
	setCond("Ready", readyErr, "AllDependenciesResolved", "DependenciesNotReady")

	newPhase := "Pending"
	if ready {
		newPhase = "Ready"
	}

	unchanged := statusUnchanged(isvc.Status.Conditions, newConditions) &&
		isvc.Status.Phase == newPhase &&
		isvc.Status.DerivedPriority == derivedPriority &&
		isvc.Status.ObservedGeneration == isvc.Generation

	if unchanged {
		return false, nil
	}

	isvc.Status.Conditions = newConditions
	isvc.Status.Phase = newPhase
	isvc.Status.DerivedPriority = derivedPriority
	isvc.Status.ObservedGeneration = isvc.Generation

	return true, r.Status().Update(ctx, isvc)
}

// mapModelToInferenceServices maps a Model change to reconcile requests
// for every InferenceService in the same namespace that references it —
// this is what makes an InferenceService notice a model becoming Ready
// immediately, instead of waiting up to 10s for the next poll.
func (r *InferenceServiceReconciler) mapModelToInferenceServices(ctx context.Context, obj client.Object) []reconcile.Request {
	model, ok := obj.(*platformv1alpha1.Model)
	if !ok {
		return nil
	}

	var list platformv1alpha1.InferenceServiceList
	if err := r.List(ctx, &list, client.InNamespace(model.Namespace)); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, isvc := range list.Items {
		if isvc.Spec.Model == model.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: isvc.Name, Namespace: isvc.Namespace},
			})
		}
	}
	return requests
}

func (r *InferenceServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1alpha1.InferenceService{}).
		Watches(
			&platformv1alpha1.Model{},
			handler.EnqueueRequestsFromMapFunc(r.mapModelToInferenceServices),
		).
		Named("inferenceservice").
		Complete(r)
}
