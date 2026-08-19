package controller

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	platformv1alpha1 "github.com/devam1402/cloud-native-inference-platform/operator/api/v1alpha1"
)

var _ = Describe("Tenant controller", func() {
	It("creates namespace and full resource quota, sets Ready", func() {
		tenant := &platformv1alpha1.Tenant{
			ObjectMeta: metav1.ObjectMeta{Name: "finance"},
			Spec: platformv1alpha1.TenantSpec{
				ResourceQuota: platformv1alpha1.TenantResourceQuota{
					CPU: "20", Memory: "64Gi", GPU: "2", Pods: "50", Jobs: "20",
				},
				Priority: platformv1alpha1.TenantPriority{Default: 100},
			},
		}
		Expect(k8sClient.Create(ctx, tenant)).To(Succeed())

		rq := &corev1.ResourceQuota{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: "tenant-quota", Namespace: "finance"}, rq)
		}).Should(Succeed())

		Expect(rq.Spec.Hard.Cpu().String()).To(Equal("20"))
		Expect(rq.Spec.Hard[corev1.ResourceName("nvidia.com/gpu")]).To(Equal(resource.MustParse("2")))

		ns := &corev1.Namespace{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "finance"}, ns)).To(Succeed())
		Expect(ns.Labels[tenantLabelKey]).To(Equal("finance"))

		Eventually(func() []metav1.Condition {
			_ = k8sClient.Get(ctx, types.NamespacedName{Name: "finance"}, tenant)
			return tenant.Status.Conditions
		}).Should(ContainElement(HaveField("Type", Equal("Ready"))))
	})

	It("is idempotent across repeated reconciles", func() {
		tenant := &platformv1alpha1.Tenant{
			ObjectMeta: metav1.ObjectMeta{Name: "idempotent-test"},
			Spec: platformv1alpha1.TenantSpec{
				ResourceQuota: platformv1alpha1.TenantResourceQuota{CPU: "10", Memory: "32Gi", GPU: "1"},
				Priority:      platformv1alpha1.TenantPriority{Default: 50},
			},
		}
		Expect(k8sClient.Create(ctx, tenant)).To(Succeed())

		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: "tenant-quota", Namespace: "idempotent-test"}, &corev1.ResourceQuota{})
		}).Should(Succeed())

		Eventually(func() error {
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: "idempotent-test"}, tenant); err != nil {
				return err
			}
			tenant.Annotations = map[string]string{"trigger": "reconcile-again"}
			return k8sClient.Update(ctx, tenant)
		}).Should(Succeed())

		Consistently(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: "tenant-quota", Namespace: "idempotent-test"}, &corev1.ResourceQuota{})
		}, "3s").Should(Succeed())
	})

	It("reconciles quota changes, not just initial provisioning", func() {
		tenant := &platformv1alpha1.Tenant{
			ObjectMeta: metav1.ObjectMeta{Name: "quota-update-test"},
			Spec: platformv1alpha1.TenantSpec{
				ResourceQuota: platformv1alpha1.TenantResourceQuota{CPU: "20", Memory: "64Gi", GPU: "2"},
				Priority:      platformv1alpha1.TenantPriority{Default: 100},
			},
		}
		Expect(k8sClient.Create(ctx, tenant)).To(Succeed())
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: "tenant-quota", Namespace: "quota-update-test"}, &corev1.ResourceQuota{})
		}).Should(Succeed())

		// Get-modify-update retried via Eventually: TenantController is
		// concurrently writing status to this same object, so a single
		// blind Update can race against it and hit a 409 Conflict on a
		// stale ResourceVersion. Retrying the whole read-modify-write
		// cycle is the standard client-go pattern for this.
		Eventually(func() error {
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: "quota-update-test"}, tenant); err != nil {
				return err
			}
			tenant.Spec.ResourceQuota.CPU = "40"
			return k8sClient.Update(ctx, tenant)
		}).Should(Succeed())

		Eventually(func() string {
			rq := &corev1.ResourceQuota{}
			_ = k8sClient.Get(ctx, types.NamespacedName{Name: "tenant-quota", Namespace: "quota-update-test"}, rq)
			return rq.Spec.Hard.Cpu().String()
		}).Should(Equal("40"))
	})
})
