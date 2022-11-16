// Copyright 2020-2021 Clastix Labs
// SPDX-License-Identifier: Apache-2.0

package v1beta1

import (
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1 "k8s.io/api/core/v1"
)

func (t *Tenant) IsCordoned() bool {
	if v, ok := t.Labels["capsule.clastix.io/cordon"]; ok && v == "enabled" {
		return true
	}

	return false
}

func (t *Tenant) IsFull() bool {
	// we don't have limits on assigned Namespaces
	if t.Spec.NamespaceOptions == nil || t.Spec.NamespaceOptions.Quota == nil {
		return false
	}

	return len(t.Status.Namespaces) >= int(*t.Spec.NamespaceOptions.Quota)
}

func (t *Tenant) AssignNamespaces(namespaces []corev1.Namespace) {
	var l []string

	for _, ns := range namespaces {
		if !t.nsBlongTenant(ns.OwnerReferences) {
			continue
		}

		if ns.Status.Phase == corev1.NamespaceActive {
			l = append(l, ns.GetName())
		}
	}

	sort.Strings(l)

	t.Status.Namespaces = l
	t.Status.Size = uint(len(l))
}

func (t *Tenant) GetOwnerProxySettings(name string, kind OwnerKind) []ProxySettings {
	return t.Spec.Owners.FindOwner(name, kind).ProxyOperations
}

func (t *Tenant) nsBlongTenant(ownerReference []metav1.OwnerReference) bool {
	if ownerReference == nil {
		return false
	}

	// Checking the namespace belongs to this tenant or not should checking ownerReference.Kind and ownerReference.Name.
	for _, or := range ownerReference {
		if or.Kind == "Tenant" && or.Name == t.Name {
			return true
		}
	}
	return false

}
