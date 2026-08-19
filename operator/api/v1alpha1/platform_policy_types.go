package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PlatformPolicySpec struct {
	// +optional
	Isolation IsolationPolicy `json:"isolation,omitempty"`

	// +optional
	SignedModels SignedModelsPolicy `json:"signedModels,omitempty"`

	// +optional
	Scheduling SchedulingPolicy `json:"scheduling,omitempty"`
}

type IsolationPolicy struct {
	// +kubebuilder:validation:Enum=standard;strict
	// +optional
	Tier string `json:"tier,omitempty"`
}

type SignedModelsPolicy struct {
	// +optional
	Required bool `json:"required,omitempty"`
}

type SchedulingPolicy struct {
	// +optional
	Preemption bool `json:"preemption,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
type PlatformPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec PlatformPolicySpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true
type PlatformPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PlatformPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PlatformPolicy{}, &PlatformPolicyList{})
}
