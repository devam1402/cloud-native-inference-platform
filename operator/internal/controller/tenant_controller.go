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
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch
// The three markers below don't correspond to anything this controller
// reads directly — they exist because reconcileRBAC creates a Role that
// grants tenants read access to pods/services/configmaps. Kubernetes'
// RBAC escalation-prevention check blocks any actor from granting
// permissions it doesn't itself hold, so the controller must hold a
// superset of whatever it writes into tenant-facing RBAC objects.
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

// Reconcile stops at the first failure only for namespace/quota, since
// nothing downstream can meaningfully exist without them. From there,
// ServiceAccount, RBAC, and NetworkPolicy are reconciled independently —
// a failure in one does not hide whether the others succeeded, so
// status conditions reflect the true state of each resource rather
// than a merged, ambiguous signal.
func (r *TenantReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var tenant platformv1alpha1.Tenant
	if err := r.Get(ctx, req.NamespacedName, &tenant); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	nsErr := r.reconcileNamespace(ctx, &tenant)
	if nsErr != nil {
		_ = r.updateStatus(ctx, &tenant, nsErr, nil, nil, nil, nil)
		return ctrl.Result{}, nsErr
	}

	quotaErr := r.reconcileResourceQuota(ctx, &tenant)
	if quotaErr != nil {
		_ = r.updateStatus(ctx, &tenant, nil, quotaErr, nil, nil, nil)
		return ctrl.Result{}, quotaErr
	}

	saErr := r.reconcileServiceAccount(ctx, &tenant)
	rbacErr := r.reconcileRBAC(ctx, &tenant)
	npErr := r.reconcileNetworkPolicy(ctx, &tenant)

	if err := r.updateStatus(ctx, &tenant, nil, nil, saErr, rbacErr, npErr); err != nil {
		return ctrl.Result{}, err
	}
	if saErr != nil {
		return ctrl.Result{}, saErr
	}
	if rbacErr != nil {
		return ctrl.Result{}, rbacErr
	}
	if npErr != nil {
		return ctrl.Result{}, npErr
	}

	log.Info("tenant reconciled", "tenant", tenant.Name)
	return ctrl.Result{}, nil
}

// updateStatus is change-aware (via statusUnchanged) and reports each
// resource's outcome as its own condition — ServiceAccountReady, RBACReady,
// and NetworkPolicyReady are independent, so kubectl describe shows exactly
// which resource is broken rather than one merged signal.
func (r *TenantReconciler) updateStatus(ctx context.Context, t *platformv1alpha1.Tenant, nsErr, quotaErr, saErr, rbacErr, npErr error) error {
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
	setCond("ServiceAccountReady", saErr, "ServiceAccountReconciled", "ServiceAccountError")
	setCond("RBACReady", rbacErr, "RBACReconciled", "RBACError")
	setCond("NetworkPolicyReady", npErr, "NetworkPolicyReconciled", "NetworkPolicyError")

	ready := nsErr == nil && quotaErr == nil && saErr == nil && rbacErr == nil && npErr == nil
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
