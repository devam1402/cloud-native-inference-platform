package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type InferenceServiceSpec struct {
	// +kubebuilder:validation:Required
	Model string `json:"model"`

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=interactive;batch;background
	WorkloadClass string `json:"workloadClass"`

	// +kubebuilder:validation:Required
	ResourceProfile string `json:"resourceProfile"`

	// +optional
	Priority *int32 `json:"priority,omitempty"`

	// +optional
	// +kubebuilder:validation:Enum=stateless;cache-affine;session-affine;long-generation
	Statefulness string `json:"statefulness,omitempty"`

	// +optional
	SLO InferenceSLO `json:"slo,omitempty"`
}

type InferenceSLO struct {
	// +optional
	TTFTP99Milliseconds *int64 `json:"ttftP99,omitempty"`

	// +optional
	TPOTP99Milliseconds *int64 `json:"tpotP99,omitempty"`

	// +optional
	Availability *string `json:"availability,omitempty"`
}

type InferenceServiceStatus struct {
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// +optional
	Phase string `json:"phase,omitempty"`

	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional
	DerivedPriority int32 `json:"derivedPriority,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Priority",type=integer,JSONPath=`.status.derivedPriority`
type InferenceService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InferenceServiceSpec   `json:"spec,omitempty"`
	Status InferenceServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type InferenceServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InferenceService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&InferenceService{}, &InferenceServiceList{})
}
