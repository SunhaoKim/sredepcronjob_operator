/*
Copyright 2021.

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
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
type ConcurrencyPolicy string

const (
	AllowConcurrent   ConcurrencyPolicy = "Allow"
	ForbidConcurrent  ConcurrencyPolicy = "Forbid"
	ReplaceConcurrent ConcurrencyPolicy = "Replace"
)

// SredepSpec defines the desired state of Sredep
type SredepSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	TaskCreator                string                   `json:"taskCreator"`
	TaskAnnotation             string                   `json:"taskannotation"`
	Schedule                   string                   `json:"schedule"`
	StartingDeadlineSeconds    *int64                   `json:"startingdeaDlineSeconds,omitempty"`
	ConcurrencyPolicy          ConcurrencyPolicy        `json:"concurrencyPolicy,omitempty"`
	Suspend                    *bool                    `json:"suspend,omitempty"`
	JobTemplate                batchv1beta1.JobTemplate `json:"jobTemplate"`
	SuccessfulJobsHistoryLimit *int32                   `json:"successfulJobsHistoryLimit,omitempty"`
	FailedJobsHistoryLimit     *int32                   `json:"failedJobsHistoryLimit,omitempty"`
	DeadlineSeconds            *int64                   `json:"deadlineSeconds,omitempty"`
}

// SredepStatus defines the observed state of Sredep
type SredepStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Active           []corev1.ObjectReference `json:"active,omitempty"`
	LastScheduleTime *metav1.Time             `json:"lastScheduleTime,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Sredep is the Schema for the sredeps API
type Sredep struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              SredepSpec   `json:"spec,omitempty"`
	Status            SredepStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SredepList contains a list of Sredep
type SredepList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Sredep `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Sredep{}, &SredepList{})
}
