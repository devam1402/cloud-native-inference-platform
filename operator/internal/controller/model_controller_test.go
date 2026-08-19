package controller

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	platformv1alpha1 "github.com/devam1402/cloud-native-inference-platform/operator/api/v1alpha1"
)

var _ = Describe("Model controller", func() {
	It("marks a model with a valid source as Ready", func() {
		model := &platformv1alpha1.Model{
			ObjectMeta: metav1.ObjectMeta{Name: "sarvam-m", Namespace: "default"},
			Spec: platformv1alpha1.ModelSpec{
				Source: platformv1alpha1.ModelSource{Type: "oci", URI: "oci://registry.example.com/sarvam-m:v1"},
			},
		}
		Expect(k8sClient.Create(ctx, model)).To(Succeed())

		Eventually(func() string {
			_ = k8sClient.Get(ctx, types.NamespacedName{Name: "sarvam-m", Namespace: "default"}, model)
			return model.Status.Phase
		}).Should(Equal("Ready"))
	})

	It("rejects a model requiring signature with no digest", func() {
		model := &platformv1alpha1.Model{
			ObjectMeta: metav1.ObjectMeta{Name: "unsigned-model", Namespace: "default"},
			Spec: platformv1alpha1.ModelSpec{
				Source:             platformv1alpha1.ModelSource{Type: "oci", URI: "oci://registry.example.com/x:v1"},
				RequireSignedImage: true,
			},
		}
		Expect(k8sClient.Create(ctx, model)).To(Succeed())

		Eventually(func() []metav1.Condition {
			_ = k8sClient.Get(ctx, types.NamespacedName{Name: "unsigned-model", Namespace: "default"}, model)
			return model.Status.Conditions
		}).Should(ContainElement(HaveField("Reason", Equal("VerificationFailed"))))
	})
})
