package tenant

import (
	"context"

	capsulev1beta1 "github.com/clastix/capsule/api/v1beta1"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// reconciles additional resources that are tied to the tenant.
// NamespaceSelector is used to select namespace for the resources to be be
// created on.
//
// Only namespaces assigned to tenant can be selected. If more than one
// namespace matches then each resource will be created in every matched
// namespace.
//
// Namespace selection is global, meaning you cant select namespace per
// resource. Matched namespaces are applied to all specified resources.
func (r *Manager) syncAdditionalResources(tenant *capsulev1beta1.Tenant) error {
	if tenant.Spec.AdditionalResources == nil {
		return nil
	}

	a := tenant.Spec.AdditionalResources
	ns := &corev1.NamespaceList{}
	labelSelector := labels.NewSelector()
	for k, v := range tenant.Spec.AdditionalResources.NamespaceSelector.MatchLabels {
		s, err := labels.NewRequirement(k, selection.In, []string{v})
		if err != nil {
			return err
		}
		labelSelector = labelSelector.Add(*s)
	}
	for _, v := range tenant.Spec.AdditionalResources.NamespaceSelector.MatchExpressions {
		s, err := labels.NewRequirement(v.Key, selection.Operator(v.Operator), v.Values)
		if err != nil {
			return err
		}
		labelSelector = labelSelector.Add(*s)
	}

	// We only select namespaces assigned to this tenant.
	capsuleLabel, _ := capsulev1beta1.GetTypeLabel(&capsulev1beta1.Tenant{})
	assignedNamespace, _ := labels.NewRequirement(capsuleLabel, selection.Equals, []string{tenant.GetName()})
	labelSelector = labelSelector.Add(*assignedNamespace)

	err := r.List(context.TODO(), ns, &client.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return client.IgnoreNotFound(err)
	}
	r.Log.Info("Creating additional resources ", "namespaces", len(ns.Items), "selector", labelSelector.String())
	createResource := func(ns string, o string) func() error {
		return func() error {
			// we can't mutate object namespace inside CreateOrUpdate. Setting the
			// namespace here ensures object will be created/updated with proper
			// namespace.
			var object unstructured.Unstructured
			err := yaml.Unmarshal([]byte(o), &object.Object)
			if err != nil {
				return err
			}
			if isRoleBinding(&object) {
				rb, err := asRoleBinding(&object)
				if err != nil {
					return err
				}
				for k, _ := range rb.Subjects {
					rb.Subjects[k].Namespace = ns
				}
				object.Object, _ = runtime.DefaultUnstructuredConverter.ToUnstructured(rb)

			}

			object.SetNamespace(ns)
			_, err = controllerutil.CreateOrUpdate(context.TODO(), r.Client, &object, func() error {
				return nil
			})
			return err
		}
	}
	errGroup := new(errgroup.Group)
	for _, n := range ns.Items {
		//  If Namespace's status is not Active should skip adding additional resources in the Namespace.
		// The additional resource like be role or rolebiding defined in Tenant resource.
		if n.Status.Phase != corev1.NamespaceActive {
			klog.Warningf("namespace [%s] cannot create addtional resource, status [%v]", n.Name, n.Status.Phase)
			continue
		}
		for _, o := range a.Items {
			errGroup.Go(createResource(n.Name, o))
		}
	}
	return errGroup.Wait()
}

func isRoleBinding(obj *unstructured.Unstructured) bool {
	return obj.GetKind() == "RoleBinding"
}

func asRoleBinding(obj *unstructured.Unstructured) (*rbac.RoleBinding, error) {
	if isNil(obj) {
		return nil, nil
	}
	var rb rbac.RoleBinding
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &rb); err != nil {
		return nil, err
	}
	return &rb, nil
}

func isNil(object runtime.Object) bool {
	if object == nil {
		return true
	}
	switch object.(type) {
	case *unstructured.Unstructured:
		obj := object.(*unstructured.Unstructured)
		if obj == nil {
			return true
		}
	}
	return false
}