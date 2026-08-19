package controller

import (
	authv1 "k8s.io/api/authorization/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	platformv1alpha1 "github.com/devam1402/cloud-native-inference-platform/operator/api/v1alpha1"
)

// canAccess runs a real SubjectAccessReview asking whether the given
// ServiceAccount identity can perform verb on resource in namespace.
// This is what actually proves authorization, as opposed to merely
// checking that a Role/RoleBinding object exists.
func canAccess(saNamespace, saName, targetNamespace, resource, verb string) bool {
	sar := &authv1.SubjectAccessReview{
		Spec: authv1.SubjectAccessReviewSpec{
			User: "system:serviceaccount:" + saNamespace + ":" + saName,
			ResourceAttributes: &authv1.ResourceAttributes{
				Namespace: targetNamespace,
				Verb:      verb,
				Group:     "platform.platform.io",
				Resource:  resource,
			},
		},
	}
	Expect(k8sClient.Create(ctx, sar)).To(Succeed())
	return sar.Status.Allowed
}

var _ = Describe("Tenant isolation — authorization", func() {
	It("proves structural isolation: roles are namespace-scoped, bindings reference local roles", func() {
		finance := &platformv1alpha1.Tenant{
			ObjectMeta: metav1.ObjectMeta{Name: "auth-finance"},
			Spec: platformv1alpha1.TenantSpec{
				ResourceQuota: platformv1alpha1.TenantResourceQuota{CPU: "10", Memory: "32Gi", GPU: "1"},
				Priority:      platformv1alpha1.TenantPriority{Default: 100},
			},
		}
		research := &platformv1alpha1.Tenant{
			ObjectMeta: metav1.ObjectMeta{Name: "auth-research"},
			Spec: platformv1alpha1.TenantSpec{
				ResourceQuota: platformv1alpha1.TenantResourceQuota{CPU: "10", Memory: "32Gi", GPU: "1"},
				Priority:      platformv1alpha1.TenantPriority{Default: 50},
			},
		}
		Expect(k8sClient.Create(ctx, finance)).To(Succeed())
		Expect(k8sClient.Create(ctx, research)).To(Succeed())

		var financeRole, researchRole rbacv1.Role
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: "tenant-workload-role", Namespace: "auth-finance"}, &financeRole)
		}).Should(Succeed())
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: "tenant-workload-role", Namespace: "auth-research"}, &researchRole)
		}).Should(Succeed())

		Expect(financeRole.Namespace).To(Equal("auth-finance"))
		Expect(researchRole.Namespace).To(Equal("auth-research"))

		var financeBinding, researchBinding rbacv1.RoleBinding
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "tenant-workload-binding", Namespace: "auth-finance"}, &financeBinding)).To(Succeed())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "tenant-workload-binding", Namespace: "auth-research"}, &researchBinding)).To(Succeed())
		Expect(financeBinding.RoleRef.Name).To(Equal("tenant-workload-role"))
		Expect(researchBinding.RoleRef.Name).To(Equal("tenant-workload-role"))
	})

	It("proves authorization isolation via SubjectAccessReview: finance cannot access research and vice versa", func() {
		finance := &platformv1alpha1.Tenant{
			ObjectMeta: metav1.ObjectMeta{Name: "sar-finance"},
			Spec: platformv1alpha1.TenantSpec{
				ResourceQuota: platformv1alpha1.TenantResourceQuota{CPU: "10", Memory: "32Gi", GPU: "1"},
				Priority:      platformv1alpha1.TenantPriority{Default: 100},
			},
		}
		research := &platformv1alpha1.Tenant{
			ObjectMeta: metav1.ObjectMeta{Name: "sar-research"},
			Spec: platformv1alpha1.TenantSpec{
				ResourceQuota: platformv1alpha1.TenantResourceQuota{CPU: "10", Memory: "32Gi", GPU: "1"},
				Priority:      platformv1alpha1.TenantPriority{Default: 50},
			},
		}
		Expect(k8sClient.Create(ctx, finance)).To(Succeed())
		Expect(k8sClient.Create(ctx, research)).To(Succeed())

		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: "tenant-workload-binding", Namespace: "sar-finance"}, &rbacv1.RoleBinding{})
		}).Should(Succeed())
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: "tenant-workload-binding", Namespace: "sar-research"}, &rbacv1.RoleBinding{})
		}).Should(Succeed())

		// the actual claim this platform makes, tested directly:
		Expect(canAccess("sar-finance", "tenant-workload", "sar-finance", "inferenceservices", "get")).To(BeTrue(), "finance SA should access its own namespace")
		Expect(canAccess("sar-finance", "tenant-workload", "sar-research", "inferenceservices", "get")).To(BeFalse(), "finance SA must NOT access research's namespace")
		Expect(canAccess("sar-research", "tenant-workload", "sar-research", "inferenceservices", "get")).To(BeTrue(), "research SA should access its own namespace")
		Expect(canAccess("sar-research", "tenant-workload", "sar-finance", "inferenceservices", "get")).To(BeFalse(), "research SA must NOT access finance's namespace")
	})
})
