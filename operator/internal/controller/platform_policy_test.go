package controller

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	platformv1alpha1 "github.com/devam1402/cloud-native-inference-platform/operator/api/v1alpha1"
)

var _ = Describe("PlatformPolicy CRD", func() {
	It("accepts a valid isolation tier", func() {
		policy := &platformv1alpha1.PlatformPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "default"},
			Spec: platformv1alpha1.PlatformPolicySpec{
				Isolation:    platformv1alpha1.IsolationPolicy{Tier: "strict"},
				SignedModels: platformv1alpha1.SignedModelsPolicy{Required: true},
				Scheduling:   platformv1alpha1.SchedulingPolicy{Preemption: true},
			},
		}
		Expect(k8sClient.Create(ctx, policy)).To(Succeed())
	})

	It("rejects an invalid isolation tier", func() {
		policy := &platformv1alpha1.PlatformPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "invalid-tier"},
			Spec: platformv1alpha1.PlatformPolicySpec{
				Isolation: platformv1alpha1.IsolationPolicy{Tier: "super-secret"},
			},
		}
		Expect(k8sClient.Create(ctx, policy)).NotTo(Succeed())
	})
})
