package controller

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	namespacelabelv1alpha1 "github.com/itayShv/namespacelabel-assignment-shv/api/v1alpha1"
)

const (
	namespaceLabelFinalizer  = "namespacelabel.dana.exam/finalizer"
	managedKeysAnnotation = "namespacelabel.dana.exam/managed-keys"
	protectedConfigMapName = "protected_labels"
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
	logger.Info("Starting reconciliation for the NameSpace Label Operator")
	
	var namespaceLabelCR namespacelabelv1alpha1.NamespaceLabel
	if err := r.Get(ctx, req.NamespacedName, &namespaceLabelCR); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var policyConfig corev1.ConfigMap
	var protectedPrefixes []string
	err := r.Get(ctx, types.NamespacedName{Name: protectedConfigMapName, Namespace: "default"}, &policyConfig)

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

		// initialize maps if missing
		if targetNamespace.Labels == nil {
			targetNamespace.Labels = make(map[string]string)
		}
		if targetNamespace.Annotations == nil {
			targetNamespace.Annotations = make(map[string]string)
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


		// delete removed labels from CR
		oldKeysString := targetNamespace.Annotations[managedKeysAnnotation]
		if oldKeysString != "" {
			oldKeys := strings.Split(oldKeysString, ",")
			for _, oldKey := range oldKeys {
				if _, exists := namespaceLabelCR.Spec.Labels[oldKey]; !exists {
					if !isProtected(oldKey) {
						logger.Info("Removing deleted label from Namespace", "key", oldKey)
						delete(targetNamespace.Labels, oldKey)
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
			targetNamespace.Labels[key] = value
			currentKeys = append(currentKeys, key)
		}

		// update tracked annotation
		targetNamespace.Annotations[managedKeysAnnotation] = strings.Join(currentKeys, ",")
		// save the Namespace
		if err := r.Update(ctx, &targetNamespace); err != nil {
			logger.Error(err, "Failed to update target Namespace labels")
			return ctrl.Result{}, err
		}

		// update CR status
		namespaceLabelCR.Status.Applied = true
		if skippedProtected {
    		namespaceLabelCR.Status.Message = "Applied labels, but skipped protected keys"
		} else {
    		namespaceLabelCR.Status.Message = "Successfully synced all labels"
		}
		if err := r.Status().Update(ctx, &namespaceLabelCR); err != nil {
			logger.Error(err, "Failed to update CR status")
			return ctrl.Result{}, err
		}

	} else {
		logger.Info("Starting Delete CR event of the namespace"+ namespaceLabelCR.Namespace)
		if controllerutil.ContainsFinalizer(&namespaceLabelCR, namespaceLabelFinalizer) {
			logger.Info("Cleanup triggered, removing managed labels")

			var targetNamespace corev1.Namespace
			if err := r.Get(ctx, types.NamespacedName{Name: namespaceLabelCR.Namespace}, &targetNamespace); err == nil {
				
				oldKeysString := targetNamespace.Annotations[managedKeysAnnotation]
				if oldKeysString != "" {
					oldKeys := strings.Split(oldKeysString, ",")
					for _, oldKey := range oldKeys {
						if !isProtected(oldKey) {
							logger.Info("Cleaning up label", "key", oldKey)
							delete(targetNamespace.Labels, oldKey)
						} else {
							logger.Info("Cannot clean up label; it is now protected", "key", oldKey)
						}
					}
				}
				
				// cleanup
				delete(targetNamespace.Annotations, managedKeysAnnotation)

				if err := r.Update(ctx, &targetNamespace); err != nil {
					logger.Error(err, "Failed to clean up target Namespace labels")
					return ctrl.Result{}, err
				}
			}

			// remove finalizer to allow Kubernetes to permanently delete the CR
			logger.Info("Removing Finalizer")
			controllerutil.RemoveFinalizer(&namespaceLabelCR, namespaceLabelFinalizer)
			if err := r.Update(ctx, &namespaceLabelCR); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}


var namespaceLabelPredicate = predicate.Funcs{
	UpdateFunc: func(e event.UpdateEvent) bool {
		// check .spec changed
		if e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() {
			return true
		}
		// check triggered deletion
		if e.ObjectOld.GetDeletionTimestamp().IsZero() && !e.ObjectNew.GetDeletionTimestamp().IsZero() {
			return true
		} else {
			return false	
		}
	},
	CreateFunc: func(e event.CreateEvent) bool { return true },
	DeleteFunc: func(e event.DeleteEvent) bool { return true },
}


// SetupWithManager sets up the controller with the Manager.
func (r *NamespaceLabelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
	// updates, only triggering when the .spec of your CR actually changes
		For(&namespacelabelv1alpha1.NamespaceLabel{}, builder.WithPredicates(namespaceLabelPredicate)).
		Named("namespacelabel").
		// drift detection: triggers only when a Namespace's labels are modified
		Watches(
			&corev1.Namespace{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Name:      obj.GetName(),
							Namespace: obj.GetName(),
						},
					},
				}
			}),
			// checks only for label changes
			builder.WithPredicates(predicate.LabelChangedPredicate{}),
		).

		Complete(r)
}
