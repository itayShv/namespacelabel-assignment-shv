/*
Copyright 2026.

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
	"context"
	"slices"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// nolint:unused
// log is for logging in this package.
var namespacelog = logf.Log.WithName("namespace-resource")

// SetupNamespaceWebhookWithManager registers the webhook for Namespace in the manager.
func SetupNamespaceWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &corev1.Namespace{}).
		WithValidator(&NamespaceCustomValidator{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: If you want to customise the 'path', use the flags '--defaulting-path' or '--validation-path'.
// +kubebuilder:webhook:path=/validate--v1-namespace,mutating=false,failurePolicy=ignore,sideEffects=None,groups="",resources=namespaces,verbs=create;update;delete,versions=v1,name=vnamespace-v1.kb.io,admissionReviewVersions=v1

// NamespaceCustomValidator struct is responsible for validating the Namespace resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type NamespaceCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Namespace.
func (v *NamespaceCustomValidator) ValidateCreate(ctx context.Context, obj *corev1.Namespace) (admission.Warnings, error) {
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		namespacelog.Error(err, "Failed to read admission request")
		return nil, nil
	}
	namespacelog.Info("Namespace created", "namespace", obj.GetName(), "user", req.UserInfo.Username)

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Namespace.
func (v *NamespaceCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj *corev1.Namespace) (admission.Warnings, error) {
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		namespacelog.Error(err, "Failed to read admission request")
		return nil, nil
	}
	addedLabels, removedLabels, changedLabels := diffLabels(oldObj.GetLabels(), newObj.GetLabels())
	namespacelog.Info("Namespace updated",
		"namespace", newObj.GetName(),
		"user", req.UserInfo.Username,
		"addedLabels", addedLabels,
		"removedLabels", removedLabels,
		"changedLabels", changedLabels,
	)

	return nil, nil
}

// diffLabels returns the label changes between two label sets as sorted strings
// ("key=value" for added/removed, "key=old->new" for changed).
// Strings are used instead of maps because label keys contain dots, which
// Elasticsearch would otherwise expand into conflicting nested fields.
func diffLabels(oldLabels, newLabels map[string]string) (added, removed, changed []string) {
	for key, newValue := range newLabels {
		oldValue, existed := oldLabels[key]
		switch {
		case !existed:
			added = append(added, key+"="+newValue)
		case oldValue != newValue:
			changed = append(changed, key+"="+oldValue+"->"+newValue)
		}
	}
	for key, oldValue := range oldLabels {
		if _, exists := newLabels[key]; !exists {
			removed = append(removed, key+"="+oldValue)
		}
	}
	slices.Sort(added)
	slices.Sort(removed)
	slices.Sort(changed)

	return added, removed, changed
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Namespace.
func (v *NamespaceCustomValidator) ValidateDelete(ctx context.Context, obj *corev1.Namespace) (admission.Warnings, error) {
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		namespacelog.Error(err, "Failed to read admission request")
		return nil, nil
	}
	namespacelog.Info("Namespace deleted", "namespace", obj.GetName(), "user", req.UserInfo.Username)

	return nil, nil
}
