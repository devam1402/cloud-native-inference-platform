package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type TenantSpec struct {
	// +kubebuilder:validation:Required
	ResourceQuota TenantResourceQuota `json:"resourceQuota"`

	// +kubebuilder:validation:Required
	Priority TenantPriority `json:"priority"`
}

type TenantResourceQuota struct {
	// +kubebuilder:validation:Pattern=`^[0-9]+(\.[0-9]+)?$`
	CPU string `json:"cpu"`

	// +kubebuilder:validation:Pattern=`^[0-9]+(Mi|Gi|Ti)$`
	Memory string `json:"memory"`

	// +kubebuilder:validation:Pattern=`^[0-9]+$`
	GPU string `json:"gpu"`

	// +optional
	// +kubebuilder:validation:Pattern=`^[0-9]+$`
	Pods string `json:"pods,omitempty"`

	// +optional
	// +kubebuilder:validation:Pattern=`^[0-9]+$`
	Jobs string `json:"jobs,omitempty"`
}

type TenantPriority struct {
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1000
	Default int32 `json:"default"`
}

type TenantStatus struct {
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// +optional
	Phase string `json:"phase,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.phase`
type Tenant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TenantSpec   `json:"spec,omitempty"`
	Status TenantStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type TenantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Tenant `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Tenant{}, &TenantList{})
}
