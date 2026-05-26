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

package v1alpha1

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	namespacelabelv1alpha1 "github.com/itayShv/namespacelabel-assignment-shv/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var namespacelabellog = logf.Log.WithName("namespacelabel-webhook")

// SetupNamespaceLabelWebhookWithManager registers the webhook for NamespaceLabel in the manager.
func SetupNamespaceLabelWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &namespacelabelv1alpha1.NamespaceLabel{}).
		WithValidator(&NamespaceLabelCustomValidator{}).
		Complete()
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: If you want to customise the 'path', use the flags '--defaulting-path' or '--validation-path'.
// +kubebuilder:webhook:path=/validate-namespacelabel-dana-exam-v1alpha1-namespacelabel,mutating=false,failurePolicy=fail,sideEffects=None,groups=namespacelabel.dana.exam,resources=namespacelabels,verbs=create;update,versions=v1alpha1,name=vnamespacelabel-v1alpha1.kb.io,admissionReviewVersions=v1

// NamespaceLabelCustomValidator struct is responsible for validating the NamespaceLabel resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type NamespaceLabelCustomValidator struct {
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type NamespaceLabel.
func (v *NamespaceLabelCustomValidator) ValidateCreate(_ context.Context, obj *namespacelabelv1alpha1.NamespaceLabel) (admission.Warnings, error) {
	namespacelabellog.Info("Validation for NamespaceLabel upon creation", "name", obj.GetName())

	if obj.GetName() != "labels" {
		err := fmt.Errorf("Rejected: To prevent state collisions, the NamespaceLabel CR must be named exactly 'labels'")
		
		// Use .Info instead of .Error to prevent the massive stack trace dump.
		// We can still print the error text using err.Error() as a key-value pair.
		namespacelabellog.Info("CR rejected due to invalid name constraint", 
			"attemptedName", obj.GetName(),
			"reason", err.Error(),
		)
		
		return nil, err
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type NamespaceLabel.
func (v *NamespaceLabelCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj *namespacelabelv1alpha1.NamespaceLabel) (admission.Warnings, error) {
	namespacelabellog.Info("Validation for NamespaceLabel upon update", "name", newObj.GetName())

	if newObj.GetName() != "labels" {
		err := fmt.Errorf("Rejected: The CR name must be exactly 'labels'. You attempted an update from '%s' to '%s'", oldObj.GetName(), newObj.GetName())

		// Broadcast both names to the Manager logs as structured JSON key-value pairs
		namespacelabellog.Info("CR update rejected due to invalid name constraint",
			"oldName", oldObj.GetName(),
			"newName", newObj.GetName(),
			"reason", err.Error(),
		)

		return nil, err
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type NamespaceLabel.
func (v *NamespaceLabelCustomValidator) ValidateDelete(_ context.Context, obj *namespacelabelv1alpha1.NamespaceLabel) (admission.Warnings, error) {
	namespacelabellog.Info("Validation for NamespaceLabel upon deletion", "name", obj.GetName())

	return nil, nil
}
