package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ModelSpec struct {
	// +kubebuilder:validation:Required
	Source ModelSource `json:"source"`

	// +optional
	Version string `json:"version,omitempty"`

	// +optional
	RequireSignedImage bool `json:"requireSignedImage,omitempty"`
}

type ModelSource struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=oci;gcs;huggingface
	Type string `json:"type"`

	// +kubebuilder:validation:Required
	URI string `json:"uri"`

	// +optional
	Digest string `json:"digest,omitempty"`
}

type ModelStatus struct {
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// +optional
	Phase string `json:"phase,omitempty"`

	// +optional
	ResolvedDigest string `json:"resolvedDigest,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.phase`
type Model struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ModelSpec   `json:"spec,omitempty"`
	Status ModelStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ModelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Model `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Model{}, &ModelList{})
}
