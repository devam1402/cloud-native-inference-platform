package controller

import (
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	platformv1alpha1 "github.com/devam1402/cloud-native-inference-platform/operator/api/v1alpha1"
	"github.com/devam1402/cloud-native-inference-platform/operator/internal/scheduling"
)

var _ = Describe("InferenceService Kueue Job integration", func() {
	It("creates a suspended, Kueue-labeled Job once Model/Tenant/Profile all resolve", func() {
		tenant := &platformv1alpha1.Tenant{
			ObjectMeta: metav1.ObjectMeta{Name: "kueue-test-tenant"},
			Spec: platformv1alpha1.TenantSpec{
				ResourceQuota: platformv1alpha1.TenantResourceQuota{CPU: "2", Memory: "4Gi", GPU: "0"},
				Priority:      platformv1alpha1.TenantPriority{Default: 100},
			},
		}
		Expect(k8sClient.Create(ctx, tenant)).To(Succeed())

		// TenantController creates the namespace asynchronously — wait for
		// it to actually exist before creating anything inside it, same
		// race this test hit without this wait.
		Eventually(func() error {
			var ns corev1.Namespace
			return k8sClient.Get(ctx, types.NamespacedName{Name: "kueue-test-tenant"}, &ns)
		}).Should(Succeed(), "TenantController should create the namespace")

		model := &platformv1alpha1.Model{
			ObjectMeta: metav1.ObjectMeta{Name: "kueue-test-model", Namespace: "kueue-test-tenant"},
			Spec: platformv1alpha1.ModelSpec{
				Source: platformv1alpha1.ModelSource{Type: "oci", URI: "example.com/m:1"},
			},
		}
		Expect(k8sClient.Create(ctx, model)).To(Succeed())

		Eventually(func() bool {
			var m platformv1alpha1.Model
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: "kueue-test-model", Namespace: "kueue-test-tenant"}, &m); err != nil {
				return false
			}
			for _, c := range m.Status.Conditions {
				if c.Type == "Ready" && c.Status == metav1.ConditionTrue {
					return true
				}
			}
			return false
		}).Should(BeTrue(), "model should reach Ready via ModelController")

		isvc := &platformv1alpha1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{Name: "kueue-test-isvc", Namespace: "kueue-test-tenant"},
			Spec: platformv1alpha1.InferenceServiceSpec{
				Model:           "kueue-test-model",
				WorkloadClass:   "interactive",
				ResourceProfile: "protected-gpu",
			},
		}
		Expect(k8sClient.Create(ctx, isvc)).To(Succeed())

		var job batchv1.Job
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: "kueue-test-isvc", Namespace: "kueue-test-tenant"}, &job)
		}).Should(Succeed(), "expected reconcileKueueJob to create a Job once all checks pass")

		Expect(job.Labels[scheduling.KueueQueueLabel]).To(Equal("kueue-test-tenant-queue"))
		Expect(job.Spec.Suspend).NotTo(BeNil())
		Expect(*job.Spec.Suspend).To(BeTrue())
		Expect(job.OwnerReferences).To(HaveLen(1))
		Expect(job.OwnerReferences[0].Name).To(Equal("kueue-test-isvc"))

		Eventually(func() string {
			var fetched platformv1alpha1.InferenceService
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: "kueue-test-isvc", Namespace: "kueue-test-tenant"}, &fetched); err != nil {
				return ""
			}
			for _, c := range fetched.Status.Conditions {
				if c.Type == "WorkloadReady" {
					return string(c.Status)
				}
			}
			return ""
		}).Should(Equal("True"), "expected WorkloadReady condition to be set True")
	})
})
