/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AlertRuleSpec defines the desired state of AlertRule
type AlertRuleSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// Alert name for the rule
	// +required
	Alert string `json:"alert"`

	// Expression for the alert rule (PromQL)
	// +required
	Expr string `json:"expr"`

	// Severity level (critical, warning, info)
	// +kubebuilder:validation:Enum=critical;warning;info
	// +optional
	Severity string `json:"severity,omitempty"`

	// Duration for which the condition must be true before alerting
	// +optional
	For string `json:"for,omitempty"`

	// Labels to add to the alert
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations for the alert
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Reference to the Deployment that triggered this alert rule
	// +optional
	DeploymentRef *DeploymentReference `json:"deploymentRef,omitempty"`
}

// DeploymentReference references a Deployment
type DeploymentReference struct {
	// Namespace of the Deployment
	// +required
	Namespace string `json:"namespace"`

	// Name of the Deployment
	// +required
	Name string `json:"name"`
}

// AlertRuleStatus defines the observed state of AlertRule.
type AlertRuleStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the AlertRule resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// AlertRule is the Schema for the alertrules API
type AlertRule struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of AlertRule
	// +required
	Spec AlertRuleSpec `json:"spec"`

	// status defines the observed state of AlertRule
	// +optional
	Status AlertRuleStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AlertRuleList contains a list of AlertRule
type AlertRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AlertRule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AlertRule{}, &AlertRuleList{})
}
