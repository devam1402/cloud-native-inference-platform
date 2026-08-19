package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	platformv1alpha1 "github.com/devam1402/cloud-native-inference-platform/operator/api/v1alpha1"
)

const tenantLabelKey = "platform.platform.io/tenant"

func (r *TenantReconciler) reconcileNamespace(ctx context.Context, tenant *platformv1alpha1.Tenant) error {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: tenant.Name}}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, ns, func() error {
		if ns.Labels == nil {
			ns.Labels = map[string]string{}
		}
		ns.Labels[tenantLabelKey] = tenant.Name
		// Pod Security Standards — enforce restricted profile per tenant
		// namespace; blocks privileged pods, host networking, etc.
		ns.Labels["pod-security.kubernetes.io/enforce"] = "restricted"
		ns.Labels["pod-security.kubernetes.io/audit"] = "restricted"
		ns.Labels["pod-security.kubernetes.io/warn"] = "restricted"
		return nil
	})
	return err
	// NOTE: Namespace is cluster-scoped, same as Tenant. SetControllerReference
	// is not used here — a cluster-scoped owner cannot cleanly own a
	// cluster-scoped Namespace the way controller-runtime's helper expects
	// for namespaced children. The tenantLabelKey label is the association
	// mechanism instead, used consistently across every object below.
}

func (r *TenantReconciler) reconcileResourceQuota(ctx context.Context, tenant *platformv1alpha1.Tenant) error {
	hard, err := buildQuotaResourceList(tenant.Spec.ResourceQuota)
	if err != nil {
		return fmt.Errorf("invalid resource quota: %w", err)
	}

	rq := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Name: "tenant-quota", Namespace: tenant.Name},
	}

	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, rq, func() error {
		if rq.Labels == nil {
			rq.Labels = map[string]string{}
		}
		rq.Labels[tenantLabelKey] = tenant.Name
		rq.Spec.Hard = hard
		return nil
	})
	return err
}

func buildQuotaResourceList(q platformv1alpha1.TenantResourceQuota) (corev1.ResourceList, error) {
	cpu, err := resource.ParseQuantity(q.CPU)
	if err != nil {
		return nil, fmt.Errorf("cpu: %w", err)
	}
	mem, err := resource.ParseQuantity(q.Memory)
	if err != nil {
		return nil, fmt.Errorf("memory: %w", err)
	}
	gpu, err := resource.ParseQuantity(q.GPU)
	if err != nil {
		return nil, fmt.Errorf("gpu: %w", err)
	}

	hard := corev1.ResourceList{
		corev1.ResourceCPU:                    cpu,
		corev1.ResourceMemory:                 mem,
		corev1.ResourceName("nvidia.com/gpu"): gpu,
	}

	if q.Pods != "" {
		pods, err := resource.ParseQuantity(q.Pods)
		if err != nil {
			return nil, fmt.Errorf("pods: %w", err)
		}
		hard[corev1.ResourcePods] = pods
	}
	if q.Jobs != "" {
		jobs, err := resource.ParseQuantity(q.Jobs)
		if err != nil {
			return nil, fmt.Errorf("jobs: %w", err)
		}
		hard[corev1.ResourceName("count/jobs.batch")] = jobs
	}

	return hard, nil
}

func (r *TenantReconciler) reconcileServiceAccount(ctx context.Context, tenant *platformv1alpha1.Tenant) error {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "tenant-workload", Namespace: tenant.Name},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, sa, func() error {
		if sa.Labels == nil {
			sa.Labels = map[string]string{}
		}
		sa.Labels[tenantLabelKey] = tenant.Name
		return nil
	})
	return err
}

// reconcileRBAC creates a Role scoped to least-privilege: full lifecycle
// control over the tenant's own InferenceService (the resource a tenant
// actually self-serves), read-only on Model (registration stays a
// platform-admin action), and read-only on pods/services/configmaps for
// workload observability. Nothing here grants cross-namespace or
// cluster-level access — namespace scoping of Role/RoleBinding is what
// makes that structurally impossible, not just a policy choice.
func (r *TenantReconciler) reconcileRBAC(ctx context.Context, tenant *platformv1alpha1.Tenant) error {
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "tenant-workload-role", Namespace: tenant.Name},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, role, func() error {
		if role.Labels == nil {
			role.Labels = map[string]string{}
		}
		role.Labels[tenantLabelKey] = tenant.Name
		role.Rules = []rbacv1.PolicyRule{
			{
				APIGroups: []string{"platform.platform.io"},
				Resources: []string{"inferenceservices"},
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			},
			{
				APIGroups: []string{"platform.platform.io"},
				Resources: []string{"models"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"pods", "services", "configmaps"},
				Verbs:     []string{"get", "list", "watch"},
			},
		}
		return nil
	})
	if err != nil {
		return err
	}

	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "tenant-workload-binding", Namespace: tenant.Name},
	}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, binding, func() error {
		if binding.Labels == nil {
			binding.Labels = map[string]string{}
		}
		binding.Labels[tenantLabelKey] = tenant.Name
		binding.RoleRef = rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "Role", Name: "tenant-workload-role"}
		binding.Subjects = []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: "tenant-workload", Namespace: tenant.Name},
		}
		return nil
	})
	return err
}

// statusUnchanged compares two condition slices for equality on the fields
// that matter for change detection (Type, Status, Reason) — used by every
// controller's updateStatus to avoid writing status when nothing changed.
func statusUnchanged(old, new []metav1.Condition) bool {
	if len(old) != len(new) {
		return false
	}
	for i := range old {
		if old[i].Type != new[i].Type || old[i].Status != new[i].Status || old[i].Reason != new[i].Reason {
			return false
		}
	}
	return true
}
