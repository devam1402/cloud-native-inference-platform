package webhook

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	platformv1alpha1 "github.com/devam1402/cloud-native-inference-platform/operator/api/v1alpha1"
	internaladmission "github.com/devam1402/cloud-native-inference-platform/operator/internal/admission"
)

var _ = Describe("InferenceService webhook", func() {
	var tenantCounter int

	// this suite runs only the webhook manager, not TenantController —
	// so unlike the live cluster (where creating a Tenant triggers the
	// controller to create its namespace), here the namespace has to be
	// created explicitly. That's intentional: this suite validates
	// webhook behavior in isolation, not the full reconcile loop, which
	// is already covered by the controller suite.
	newTenantName := func() string {
		tenantCounter++
		return fmt.Sprintf("wh-tenant-%d", tenantCounter)
	}

	createTenantAndNamespace := func(name string, priority int32) {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		tenant := &platformv1alpha1.Tenant{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: platformv1alpha1.TenantSpec{
				ResourceQuota: platformv1alpha1.TenantResourceQuota{CPU: "1", Memory: "1Gi", GPU: "0"},
				Priority:      platformv1alpha1.TenantPriority{Default: priority},
			},
		}
		Expect(k8sClient.Create(ctx, tenant)).To(Succeed())
	}

	It("allows a valid InferenceService referencing an existing Tenant", func() {
		tenantName := newTenantName()
		createTenantAndNamespace(tenantName, 100)

		isvc := &platformv1alpha1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{Name: "valid", Namespace: tenantName},
			Spec: platformv1alpha1.InferenceServiceSpec{
				Model:           "m1",
				WorkloadClass:   "interactive",
				ResourceProfile: "protected-gpu",
			},
		}
		Expect(k8sClient.Create(ctx, isvc)).To(Succeed())
	})

	It("denies an InferenceService whose Tenant does not exist", func() {
		// namespace exists (so the API server accepts the create at all),
		// but no Tenant object — this is the actual condition the
		// webhook's validateTenantExists check is meant to catch.
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "no-such-tenant-namespace"}}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		isvc := &platformv1alpha1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{Name: "orphan", Namespace: "no-such-tenant-namespace"},
			Spec: platformv1alpha1.InferenceServiceSpec{
				Model:           "m1",
				WorkloadClass:   "interactive",
				ResourceProfile: "protected-gpu",
			},
		}
		err := k8sClient.Create(ctx, isvc)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("TENANT_NOT_FOUND"))
	})

	It("denies an InferenceService with an invalid resource profile", func() {
		tenantName := newTenantName()
		createTenantAndNamespace(tenantName, 100)

		isvc := &platformv1alpha1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{Name: "bad-profile", Namespace: tenantName},
			Spec: platformv1alpha1.InferenceServiceSpec{
				Model:           "m1",
				WorkloadClass:   "interactive",
				ResourceProfile: "does-not-exist",
			},
		}
		err := k8sClient.Create(ctx, isvc)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("PROFILE_INVALID"))
	})

	It("injects tenant identity and derived priority annotations on create", func() {
		tenantName := newTenantName()
		createTenantAndNamespace(tenantName, 90)

		isvc := &platformv1alpha1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{Name: "mutated", Namespace: tenantName},
			Spec: platformv1alpha1.InferenceServiceSpec{
				Model:           "m1",
				WorkloadClass:   "batch",
				ResourceProfile: "batch-shared",
			},
		}
		Expect(k8sClient.Create(ctx, isvc)).To(Succeed())

		var fetched platformv1alpha1.InferenceService
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "mutated", Namespace: tenantName}, &fetched)).To(Succeed())

		Expect(fetched.Annotations[internaladmission.AnnotationTenant]).To(Equal(tenantName))
		// batch workload class: priority.Calculate divides by 3 -> 90/3 = 30
		Expect(fetched.Annotations[internaladmission.AnnotationDerivedPriority]).To(Equal("30"))
	})
})
