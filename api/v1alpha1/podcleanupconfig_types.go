/*
Copyright 2022.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DeletionMode is the specified deletion mode for the pod-cleanup-operator
// +kubebuilder:validation:Enum=Normal;Force;Skip
type DeletionMode string

const (
	// DeletionModeNormal attempts normal deletion of bad or completed pod
	DeletionModeNormal DeletionMode = "Normal"
	// DeletionModeForce attempts forceful deletion of bad or completed pod
	DeletionModeForce DeletionMode = "Force"
	// DeletionModeSkip skips deletion of bad or completed pod
	DeletionModeSkip DeletionMode = "Skip"
)

// PodCleanupConfigSpec defines the desired state of PodCleanupConfig
type PodCleanupConfigSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// DeletionMode is the specified deletion mode for the pod-cleanup-operator
	// +kubebuilder:validation:Required
	DeletionMode DeletionMode `json:"deletionMode"`

	// DeletionDelayInMinutes is the time interval specified in minutes
	// If the pod is in the any of the bad or completed state for more minutes
	// than specified by DeletionDelay field, it is deleted
	// +kubebuilder:validation:Type=integer
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum:=0
	DeletionDelayInMinutes int32 `json:"deletionDelayInMinutes"`
}

// PodCleanupConfigStatus defines the observed state of PodCleanupConfig
type PodCleanupConfigStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// PodCleanupConfig is the Schema for the podcleanupconfigs API
type PodCleanupConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PodCleanupConfigSpec   `json:"spec,omitempty"`
	Status PodCleanupConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PodCleanupConfigList contains a list of PodCleanupConfig
type PodCleanupConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PodCleanupConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PodCleanupConfig{}, &PodCleanupConfigList{})
}
