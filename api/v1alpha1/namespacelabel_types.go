package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NamespaceLabelSpec defines the desired state of NamespaceLabel
type NamespaceLabelSpec struct {
	// Labels is the map of key-value pairs that should be applied to the Namespace
	// +kubebuilder:validation:Required
	Labels map[string]string `json:"labels"`
}

// NamespaceLabelStatus defines the observed state of NamespaceLabel.
type NamespaceLabelStatus struct {
	// Applied indicates if the labels were successfully synced to the Namespace
	Applied bool `json:"applied"`

	// Message provides details about the sync status, such as failure reasons
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Applied",type="boolean",JSONPath=".status.applied",description="Are labels applied"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.message",description="Status message"

// NamespaceLabel is the Schema for the namespacelabels API
type NamespaceLabel struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of NamespaceLabel
	// +required
	Spec NamespaceLabelSpec `json:"spec"`

	// status defines the observed state of NamespaceLabel
	// +optional
	Status NamespaceLabelStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// NamespaceLabelList contains a list of NamespaceLabel
type NamespaceLabelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []NamespaceLabel `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NamespaceLabel{}, &NamespaceLabelList{})
}
