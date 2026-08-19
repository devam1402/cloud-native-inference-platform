package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	platformv1alpha1 "github.com/devam1402/cloud-native-inference-platform/operator/api/v1alpha1"
)

// TenantReconciler reconciles a Tenant object
type TenantReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=platform.platform.io,resources=tenants,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=platform.platform.io,resources=tenants/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=resourcequotas,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch

// Reconcile runs each step sequentially and stops at the first failure —
// namespace must succeed before quota is attempted, quota before RBAC,
// so a namespace failure never cascades into secondary "namespace not
// found" errors from every step downstream of it.
func (r *TenantReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var tenant platformv1alpha1.Tenant
	if err := r.Get(ctx, req.NamespacedName, &tenant); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	nsErr := r.reconcileNamespace(ctx, &tenant)
	if nsErr != nil {
		_ = r.updateStatus(ctx, &tenant, nsErr, nil, nil, nil)
		return ctrl.Result{}, nsErr
	}

	quotaErr := r.reconcileResourceQuota(ctx, &tenant)
	if quotaErr != nil {
		_ = r.updateStatus(ctx, &tenant, nil, quotaErr, nil, nil)
		return ctrl.Result{}, quotaErr
	}

	saErr := r.reconcileServiceAccount(ctx, &tenant)

	// first-failure only, not concatenated — a single clear reason
	// instead of two error strings smashed together
	rbacErr := saErr
	if rbacErr == nil {
		rbacErr = r.reconcileRBAC(ctx, &tenant)
	}

	if err := r.updateStatus(ctx, &tenant, nil, nil, saErr, rbacErr); err != nil {
		return ctrl.Result{}, err
	}
	if saErr != nil {
		return ctrl.Result{}, saErr
	}
	if rbacErr != nil {
		return ctrl.Result{}, rbacErr
	}

	log.Info("tenant reconciled", "tenant", tenant.Name)
	return ctrl.Result{}, nil
}

// updateStatus is change-aware (via statusUnchanged, in controller_helpers.go)
// and called at each failure point above with only the relevant error set —
// nsErr/quotaErr/saErr/rbacErr default to nil for steps not yet attempted,
// so partial reconciles report exactly what's known so far, not a false
// pass on unattempted steps.
func (r *TenantReconciler) updateStatus(ctx context.Context, t *platformv1alpha1.Tenant, nsErr, quotaErr, saErr, rbacErr error) error {
	newConditions := append([]metav1.Condition{}, t.Status.Conditions...)

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

	setCond("NamespaceReady", nsErr, "NamespaceReconciled", "NamespaceError")
	setCond("QuotaApplied", quotaErr, "QuotaReconciled", "QuotaError")
	setCond("RBACReady", rbacErr, "RBACReconciled", "RBACError")

	ready := nsErr == nil && quotaErr == nil && saErr == nil && rbacErr == nil
	setCond("Accepted", nil, "TenantObserved", "")

	var readyErr error
	if !ready {
		readyErr = fmt.Errorf("not all resources reconciled")
	}
	setCond("Ready", readyErr, "AllResourcesReconciled", "ResourcesNotReady")

	newPhase := "Pending"
	if ready {
		newPhase = "Ready"
	}

	if statusUnchanged(t.Status.Conditions, newConditions) && t.Status.Phase == newPhase {
		return nil
	}

	t.Status.Conditions = newConditions
	t.Status.Phase = newPhase
	return r.Status().Update(ctx, t)
}

func (r *TenantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1alpha1.Tenant{}).
		Named("tenant").
		Complete(r)
}
