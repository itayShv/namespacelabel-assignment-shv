package controller

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	namespacelabelv1alpha1 "github.com/itayShv/namespacelabel-assignment-shv/api/v1alpha1"
)

const (
	namespaceLabelFinalizer = "namespacelabel.dana.exam/finalizer"
	managedKeysAnnotation   = "namespacelabel.dana.exam/managed-keys"
	protectedConfigMapName  = "protected-labels"
	protectedConfigLocation = "default"
)

// NamespaceLabelReconciler reconciles a NamespaceLabel object
type NamespaceLabelReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=namespacelabel.dana.exam,resources=namespacelabels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=namespacelabel.dana.exam,resources=namespacelabels/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=namespacelabel.dana.exam,resources=namespacelabels/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch

func (r *NamespaceLabelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	var namespaceLabelCR namespacelabelv1alpha1.NamespaceLabel
	if err := r.Get(ctx, req.NamespacedName, &namespaceLabelCR); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	// TODO: For a production deployment, this Singleton validation should be
	// moved to a Validating Admission Webhook to prevent etcd bloat.

	if namespaceLabelCR.Name != "labels" {
		logger.Info("Rejected CR: To prevent state collisions, the NamespaceLabel CR must be named exactly 'labels'.",
			"GotName", namespaceLabelCR.Name,
			"Namespace", namespaceLabelCR.Namespace)

		return ctrl.Result{}, nil
	}
	logger.Info("Starting reconciliation for the NameSpace Label Operator")
	var policyConfig corev1.ConfigMap
	var protectedPrefixes []string
	err := r.Get(ctx, types.NamespacedName{Name: protectedConfigMapName, Namespace: protectedConfigLocation}, &policyConfig)

	if err == nil {
		prefixesString := policyConfig.Data["protected-prefixes"]
		protectedPrefixes = strings.Split(prefixesString, ",")
	} else {
		logger.Info("Policy ConfigMap not found, checking default protected labels")
		protectedPrefixes = []string{"kubernetes.io/", "k8s.io/"}
	}

	isProtected := func(key string) bool {
		for _, prefix := range protectedPrefixes {
			if strings.HasPrefix(key, prefix) {
				return true
			}
		}
		return false
	}

	// check if the delete CR got triggered
	if namespaceLabelCR.ObjectMeta.DeletionTimestamp.IsZero() {
		// handle create or edit event

		// add finalizer if missing
		if !controllerutil.ContainsFinalizer(&namespaceLabelCR, namespaceLabelFinalizer) {
			logger.Info("Adding Finalizer to CR")
			controllerutil.AddFinalizer(&namespaceLabelCR, namespaceLabelFinalizer)
			if err := r.Update(ctx, &namespaceLabelCR); err != nil {
				return ctrl.Result{}, err
			}
		}

		// fetch the target Kubernetes Namespace
		var targetNamespace corev1.Namespace

		if err := r.Get(ctx, types.NamespacedName{Name: namespaceLabelCR.Namespace}, &targetNamespace); err != nil {
			logger.Error(err, "Failed to get target Namespace", "namespace", namespaceLabelCR.Namespace)

			namespaceLabelCR.Status.Applied = false
			namespaceLabelCR.Status.Message = "Failed to find target Namespace"
			_ = r.Status().Update(ctx, &namespaceLabelCR)

			return ctrl.Result{}, err
		}

		// initialize maps if missing
		if targetNamespace.Labels == nil {
			targetNamespace.Labels = make(map[string]string)
		}
		if targetNamespace.Annotations == nil {
			targetNamespace.Annotations = make(map[string]string)
		}

		// delete removed labels from CR
		needsUpdate := false

		oldKeysString := targetNamespace.Annotations[managedKeysAnnotation]
		if oldKeysString != "" {
			oldKeys := strings.Split(oldKeysString, ",")
			for _, oldKey := range oldKeys {
				if _, exists := namespaceLabelCR.Spec.Labels[oldKey]; !exists {
					if isProtected(oldKey) {
						logger.Info("Cannot delete label; it is protected", "key", oldKey)
						continue
					}
					// checking if it exists
					if _, hasLabel := targetNamespace.Labels[oldKey]; hasLabel {
						logger.Info("Removing deleted label from Namespace", "key", oldKey)
						delete(targetNamespace.Labels, oldKey)
						needsUpdate = true
					}
				}
			}
		}

		// add or update current labels from CR
		var currentKeys []string
		skippedProtected := false

		for key, value := range namespaceLabelCR.Spec.Labels {
			if isProtected(key) {
				logger.Info("Skipping protected label in CR", "key", key)
				skippedProtected = true
				continue
			}
			// Only trigger update if the label doesn't exist or the value is different
			if targetNamespace.Labels[key] != value {
				targetNamespace.Labels[key] = value
				needsUpdate = true
			}
			currentKeys = append(currentKeys, key)
		}

		// update tracked annotation
		newManagedKeysStr := strings.Join(currentKeys, ",")
		if targetNamespace.Annotations[managedKeysAnnotation] != newManagedKeysStr {
			targetNamespace.Annotations[managedKeysAnnotation] = newManagedKeysStr
			needsUpdate = true
		}

		// save the Namespace
		if !needsUpdate {
			logger.Info("Namespace is already in the desired state.", "namespace", targetNamespace.Name)
		} else {
			if err := r.Update(ctx, &targetNamespace); err != nil {
				logger.Error(err, "Failed to update target Namespace labels")
				return ctrl.Result{}, err
			}
			logger.Info("Successfully updated labels", "namespace", targetNamespace.Name)
		}

		// update CR status
		newApplied := true
		newMessage := "Successfully synced all labels"
		if skippedProtected {
			newMessage = "Applied labels, but skipped protected keys"
		}

		// only make the API call if the status actually needs to change
		if namespaceLabelCR.Status.Applied != newApplied || namespaceLabelCR.Status.Message != newMessage {
			namespaceLabelCR.Status.Applied = newApplied
			namespaceLabelCR.Status.Message = newMessage

			if err := r.Status().Update(ctx, &namespaceLabelCR); err != nil {
				logger.Error(err, "Failed to update CR status")
				return ctrl.Result{}, err
			}
		}
	} else {
		logger.Info("Starting Delete CR event of the namespace" + namespaceLabelCR.Namespace)

		if controllerutil.ContainsFinalizer(&namespaceLabelCR, namespaceLabelFinalizer) {
			logger.Info("Cleanup triggered, removing managed labels")

			var targetNamespace corev1.Namespace
			if err := r.Get(ctx, types.NamespacedName{Name: namespaceLabelCR.Namespace}, &targetNamespace); err == nil {
				needsUpdate := false

				oldKeysString := targetNamespace.Annotations[managedKeysAnnotation]
				if oldKeysString != "" {
					oldKeys := strings.Split(oldKeysString, ",")
					for _, oldKey := range oldKeys {
						if isProtected(oldKey) {
							logger.Info("Cannot clean up label; it is protected", "key", oldKey)
							continue
						}
						// checking if it exists
						if _, hasLabel := targetNamespace.Labels[oldKey]; hasLabel {
							logger.Info("Cleaning up label", "key", oldKey)
							delete(targetNamespace.Labels, oldKey)
							needsUpdate = true
						}
					}
				}

				// cleanup
				if _, hasAnnotation := targetNamespace.Annotations[managedKeysAnnotation]; hasAnnotation {
					delete(targetNamespace.Annotations, managedKeysAnnotation)
					needsUpdate = true
				}

				if needsUpdate {
					if err := r.Update(ctx, &targetNamespace); err != nil {
						logger.Error(err, "Failed to clean up target Namespace labels")
						return ctrl.Result{}, err
					}
					logger.Info("Successfully cleaned up Namespace labels")
				} else {
					logger.Info("Namespace is already clean. Skipping API update.")
				}
			}

			// remove finalizer to allow Kubernetes to permanently delete the CR
			logger.Info("Removing Finalizer")
			controllerutil.RemoveFinalizer(&namespaceLabelCR, namespaceLabelFinalizer)
			if err := r.Update(ctx, &namespaceLabelCR); err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, nil
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespaceLabelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// only triggering when the .spec of the CR actually changes
		For(&namespacelabelv1alpha1.NamespaceLabel{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Named("namespacelabel").

		// drift detection: triggers only when a Namespace's labels are modified
		Watches(
			&corev1.Namespace{},
			handler.EnqueueRequestsFromMapFunc(r.findCustomResourcesInNamespace),
			builder.WithPredicates(r.namespaceLabelDriftPredicate()),
		).
		Complete(r)
}

func (r *NamespaceLabelReconciler) namespaceLabelDriftPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldNs, okOld := e.ObjectOld.(*corev1.Namespace)
			newNs, okNew := e.ObjectNew.(*corev1.Namespace)
			if !okOld || !okNew {
				return false
			}

			// Read the annotation YOUR Reconcile loop wrote!
			managedKeysStr := newNs.GetAnnotations()[managedKeysAnnotation]
			if managedKeysStr == "" {
				return false
			}

			keys := strings.Split(managedKeysStr, ",")
			for _, key := range keys {
				if oldNs.GetLabels()[key] != newNs.GetLabels()[key] {
					return true
				}
			}
			return false
		},
		CreateFunc:  func(e event.CreateEvent) bool { return false },
		DeleteFunc:  func(e event.DeleteEvent) bool { return false },
		GenericFunc: func(e event.GenericEvent) bool { return false },
	}
}

func (r *NamespaceLabelReconciler) findCustomResourcesInNamespace(ctx context.Context, obj client.Object) []reconcile.Request {
	ns, ok := obj.(*corev1.Namespace)
	if !ok {
		return nil
	}

	var crList namespacelabelv1alpha1.NamespaceLabelList
	// List ALL NamespaceLabel CRs in this namespace, regardless of what they are named
	if err := r.List(ctx, &crList, client.InNamespace(ns.Name)); err != nil || len(crList.Items) == 0 {
		return nil
	}

	var requests []reconcile.Request
	for _, cr := range crList.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      cr.Name,
				Namespace: cr.Namespace,
			},
		})
	}
	return requests
}
