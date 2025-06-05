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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PVCResizeSpec defines the desired state of PVCResize
type PVCResizeSpec struct {
	// Resizing policies
	Policies []PVCResizePolicy `json:"policies,omitempty"`
}

type PVCResizePolicy struct {
	// Name of the PVC
	Ref string `json:"ref"`

	// Size of the pvc
	Size string `json:"size"`
}

// PVCResizeStatus defines the observed state of PVCResize
type PVCResizeStatus struct {
	//+optional
	PVCs []ResizeStatus `json:"pvcs"`
	//+optional
	Phase string `json:"phase,omitempty"`
	//+optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Show the list of pvcs and pvs at the status
	//+optional
	PVCNames []string `json:"pvcNames"`

	//+optional
	PVNames []string `json:"pvNames"`
}

type ResizeStatus struct {
	PVCName      string `json:"pvcName"`
	PVName       string `json:"pvName"`
	ResizeStatus string `json:"ResizeStatus"`
	Size         string `json:"size"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name="Persistent-Volumes",type=string,JSONPath=`.status.pvNames`
//+kubebuilder:printcolumn:name="PVCs",type=string,JSONPath=`.status.pvcNames`

// PVCResize is the Schema for the pvcresizes API
type PVCResize struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PVCResizeSpec   `json:"spec,omitempty"`
	Status PVCResizeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PVCResizeList contains a list of PVCResize
type PVCResizeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PVCResize `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PVCResize{}, &PVCResizeList{})
}
