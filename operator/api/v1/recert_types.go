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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type RecertSpec struct {
	Domain          string `json:"domain"`
	Email           string `json:"email"`
	SslReverseProxy string `json:"sslReverseProxy"`
}

// CertStatus defines the observed state of Cert
type RecertStatus struct {
	State           string `json:"state,omitempty"`
	LastUpdated     string `json:"lastUpdated,omitempty"`
	LastStateChange int64  `json:"lastStateChange,omitempty"`
}

// CertState is the state of the Cert
type CertState string

const (

	// Pending when we are waiting  for a chance to do something
	Pending = "Pending"

	// FailureBackoff after a failure there is a backoff period
	FailureBackoff = "FailureBackoff"

	// Creating when the Cert is first being created by certbot
	Creating = "Creating"

	// Updated when the Cert has been updated by certbot
	Updated = "Updated"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Recert is the Schema for the recerts API
type Recert struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RecertSpec   `json:"spec,omitempty"`
	Status RecertStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RecertList contains a list of Recert
type RecertList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Recert `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Recert{}, &RecertList{})
}
