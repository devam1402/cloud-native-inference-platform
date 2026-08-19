package controller

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	platformv1alpha1 "github.com/devam1402/cloud-native-inference-platform/operator/api/v1alpha1"
)

// waitForTenantNamespace waits for TenantController to actually finish
// reconciling — i.e. for the Namespace to exist — not just for the Tenant
// object to be readable back (which is trivially true right after Create).
func waitForTenantNamespace(name string) {
	Eventually(func() error {
		return k8sClient.Get(ctx, types.NamespacedName{Name: name}, &corev1.Namespace{})
	}).Should(Succeed())
}

var _ = Describe("InferenceService controller", func() {
	It("becomes Ready when all inference dependencies resolve", func() {
		tenant := &platformv1alpha1.Tenant{
			ObjectMeta: metav1.ObjectMeta{Name: "isvc-finance"},
			Spec: platformv1alpha1.TenantSpec{
				ResourceQuota: platformv1alpha1.TenantResourceQuota{CPU: "20", Memory: "64Gi", GPU: "2"},
				Priority:      platformv1alpha1.TenantPriority{Default: 100},
			},
		}
		Expect(k8sClient.Create(ctx, tenant)).To(Succeed())
		waitForTenantNamespace("isvc-finance")

		model := &platformv1alpha1.Model{
			ObjectMeta: metav1.ObjectMeta{Name: "qwen-7b", Namespace: "isvc-finance"},
			Spec:       platformv1alpha1.ModelSpec{Source: platformv1alpha1.ModelSource{Type: "oci", URI: "oci://registry.example.com/qwen-7b:v1"}},
		}
		Expect(k8sClient.Create(ctx, model)).To(Succeed())
		Eventually(func() string {
			_ = k8sClient.Get(ctx, types.NamespacedName{Name: "qwen-7b", Namespace: "isvc-finance"}, model)
			return model.Status.Phase
		}).Should(Equal("Ready"))

		isvc := &platformv1alpha1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{Name: "finance-qwen", Namespace: "isvc-finance"},
			Spec: platformv1alpha1.InferenceServiceSpec{
				Model: "qwen-7b", WorkloadClass: "interactive", ResourceProfile: "protected-gpu", Statefulness: "cache-affine",
			},
		}
		Expect(k8sClient.Create(ctx, isvc)).To(Succeed())

		Eventually(func() string {
			_ = k8sClient.Get(ctx, types.NamespacedName{Name: "finance-qwen", Namespace: "isvc-finance"}, isvc)
			return isvc.Status.Phase
		}).Should(Equal("Ready"))

		Expect(isvc.Status.DerivedPriority).To(Equal(int32(100)))
		Expect(isvc.Status.ObservedGeneration).To(Equal(isvc.Generation))
	})

	It("stays Pending when the resource profile is unknown", func() {
		tenant := &platformv1alpha1.Tenant{
			ObjectMeta: metav1.ObjectMeta{Name: "isvc-badprofile"},
			Spec: platformv1alpha1.TenantSpec{
				ResourceQuota: platformv1alpha1.TenantResourceQuota{CPU: "10", Memory: "32Gi", GPU: "1"},
				Priority:      platformv1alpha1.TenantPriority{Default: 50},
			},
		}
		Expect(k8sClient.Create(ctx, tenant)).To(Succeed())
		waitForTenantNamespace("isvc-badprofile")

		model := &platformv1alpha1.Model{
			ObjectMeta: metav1.ObjectMeta{Name: "qwen-7b", Namespace: "isvc-badprofile"},
			Spec:       platformv1alpha1.ModelSpec{Source: platformv1alpha1.ModelSource{Type: "oci", URI: "oci://x:v1"}},
		}
		Expect(k8sClient.Create(ctx, model)).To(Succeed())

		isvc := &platformv1alpha1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{Name: "bad-profile-svc", Namespace: "isvc-badprofile"},
			Spec:       platformv1alpha1.InferenceServiceSpec{Model: "qwen-7b", WorkloadClass: "interactive", ResourceProfile: "does-not-exist"},
		}
		Expect(k8sClient.Create(ctx, isvc)).To(Succeed())

		Eventually(func() []metav1.Condition {
			_ = k8sClient.Get(ctx, types.NamespacedName{Name: "bad-profile-svc", Namespace: "isvc-badprofile"}, isvc)
			return isvc.Status.Conditions
		}).Should(ContainElement(HaveField("Reason", Equal("ProfileNotFound"))))
	})

	It("stays Pending while the model has not verified yet, then becomes Ready once it does", func() {
		tenant := &platformv1alpha1.Tenant{
			ObjectMeta: metav1.ObjectMeta{Name: "isvc-modelwait"},
			Spec: platformv1alpha1.TenantSpec{
				ResourceQuota: platformv1alpha1.TenantResourceQuota{CPU: "10", Memory: "32Gi", GPU: "1"},
				Priority:      platformv1alpha1.TenantPriority{Default: 60},
			},
		}
		Expect(k8sClient.Create(ctx, tenant)).To(Succeed())
		waitForTenantNamespace("isvc-modelwait")

		model := &platformv1alpha1.Model{
			ObjectMeta: metav1.ObjectMeta{Name: "slow-model", Namespace: "isvc-modelwait"},
			Spec: platformv1alpha1.ModelSpec{
				Source:             platformv1alpha1.ModelSource{Type: "oci", URI: "oci://x:v1"},
				RequireSignedImage: true,
			},
		}
		Expect(k8sClient.Create(ctx, model)).To(Succeed())

		isvc := &platformv1alpha1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{Name: "waits-on-model", Namespace: "isvc-modelwait"},
			Spec:       platformv1alpha1.InferenceServiceSpec{Model: "slow-model", WorkloadClass: "interactive", ResourceProfile: "protected-gpu"},
		}
		Expect(k8sClient.Create(ctx, isvc)).To(Succeed())

		Eventually(func() []metav1.Condition {
			_ = k8sClient.Get(ctx, types.NamespacedName{Name: "waits-on-model", Namespace: "isvc-modelwait"}, isvc)
			return isvc.Status.Conditions
		}).Should(ContainElement(HaveField("Reason", Equal("ModelNotReady"))))

		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "slow-model", Namespace: "isvc-modelwait"}, model)).To(Succeed())
		model.Spec.Source.Digest = "sha256:deadbeef"
		Expect(k8sClient.Update(ctx, model)).To(Succeed())

		Eventually(func() string {
			_ = k8sClient.Get(ctx, types.NamespacedName{Name: "waits-on-model", Namespace: "isvc-modelwait"}, isvc)
			return isvc.Status.Phase
		}).Should(Equal("Ready"))
	})

	It("stays Pending when the tenant does not exist", func() {
		isvc := &platformv1alpha1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{Name: "orphan-svc", Namespace: "default"},
			Spec:       platformv1alpha1.InferenceServiceSpec{Model: "whatever", WorkloadClass: "interactive", ResourceProfile: "protected-gpu"},
		}
		Expect(k8sClient.Create(ctx, isvc)).To(Succeed())

		Eventually(func() []metav1.Condition {
			_ = k8sClient.Get(ctx, types.NamespacedName{Name: "orphan-svc", Namespace: "default"}, isvc)
			return isvc.Status.Conditions
		}).Should(ContainElement(HaveField("Reason", Equal("TenantNotFound"))))
	})

	It("wires priority derivation into the controller for each workload class", func() {
		tenant := &platformv1alpha1.Tenant{
			ObjectMeta: metav1.ObjectMeta{Name: "isvc-priority"},
			Spec: platformv1alpha1.TenantSpec{
				ResourceQuota: platformv1alpha1.TenantResourceQuota{CPU: "10", Memory: "32Gi", GPU: "1"},
				Priority:      platformv1alpha1.TenantPriority{Default: 90},
			},
		}
		Expect(k8sClient.Create(ctx, tenant)).To(Succeed())
		waitForTenantNamespace("isvc-priority")

		model := &platformv1alpha1.Model{
			ObjectMeta: metav1.ObjectMeta{Name: "m1", Namespace: "isvc-priority"},
			Spec:       platformv1alpha1.ModelSpec{Source: platformv1alpha1.ModelSource{Type: "oci", URI: "oci://x:v1"}},
		}
		Expect(k8sClient.Create(ctx, model)).To(Succeed())
		Eventually(func() string {
			_ = k8sClient.Get(ctx, types.NamespacedName{Name: "m1", Namespace: "isvc-priority"}, model)
			return model.Status.Phase
		}).Should(Equal("Ready"))

		isvc := &platformv1alpha1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{Name: "batch-svc", Namespace: "isvc-priority"},
			Spec:       platformv1alpha1.InferenceServiceSpec{Model: "m1", WorkloadClass: "batch", ResourceProfile: "batch-shared"},
		}
		Expect(k8sClient.Create(ctx, isvc)).To(Succeed())

		Eventually(func() int32 {
			_ = k8sClient.Get(ctx, types.NamespacedName{Name: "batch-svc", Namespace: "isvc-priority"}, isvc)
			return isvc.Status.DerivedPriority
		}).Should(Equal(int32(30)))
	})
})
