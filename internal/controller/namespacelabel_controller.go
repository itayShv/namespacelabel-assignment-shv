package controller

import (
	"context"
	"sort"
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
	namespaceLabelFinalizer     = "namespacelabel.dana.exam/finalizer"
	managedKeysAnnotation       = "namespacelabel.dana.exam/managed-keys"
	protectedConfigMapName      = "protected-labels"
	protectedConfigLocation     = "default"
	defaultProtectedPrefixesStr = "kubernetes.io/,k8s.io/"
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

	logger.Info("Starting reconciliation for the NameSpace Label Operator")
	protectedPrefixes := r.getProtectedPrefixes(ctx)

	if !namespaceLabelCR.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.handleDelete(ctx, &namespaceLabelCR, protectedPrefixes)
	}

	return r.handleUpdate(ctx, &namespaceLabelCR, protectedPrefixes)
}

func (r *NamespaceLabelReconciler) handleUpdate(ctx context.Context, cr *namespacelabelv1alpha1.NamespaceLabel, protectedPrefixes []string) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	// handle create or edit event

	// add finalizer if missing
	if !controllerutil.ContainsFinalizer(cr, namespaceLabelFinalizer) {
		logger.Info("Adding Finalizer to CR")
		controllerutil.AddFinalizer(cr, namespaceLabelFinalizer)
		if err := r.Update(ctx, cr); err != nil {
			return ctrl.Result{}, err
		}

		// ResourceVersion has changed . immediately requeue with the fresh object
		return ctrl.Result{Requeue: true}, nil
	}

	// fetch the target Kubernetes Namespace
	var targetNamespace corev1.Namespace
	if err := r.Get(ctx, types.NamespacedName{Name: cr.Namespace}, &targetNamespace); err != nil {
		logger.Error(err, "Failed to get target Namespace", "namespace", cr.Namespace)
		cr.Status.Applied = false
		cr.Status.Message = "Failed to find target Namespace"
		_ = r.Status().Update(ctx, cr)

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
	needsCleanupUpdate := r.removeStaleLabels(ctx, &targetNamespace, protectedPrefixes, cr.Spec.Labels)

	// add or update current labels from CR
	needsApplyUpdate, currentKeys, skippedProtected := r.applyNewLabels(ctx, &targetNamespace, protectedPrefixes, cr.Spec.Labels)

	// update tracked annotation
	sort.Strings(currentKeys)
	newManagedKeysStr := strings.Join(currentKeys, ",")

	if targetNamespace.Annotations[managedKeysAnnotation] != newManagedKeysStr {
		targetNamespace.Annotations[managedKeysAnnotation] = newManagedKeysStr
		needsCleanupUpdate = true // trigger the Namespace update flag
	}

	// save the Namespace
	if !needsCleanupUpdate && !needsApplyUpdate {
		logger.Info("Namespace is already in the desired state.", "namespace", targetNamespace.Name)
	} else {
		if err := r.Update(ctx, &targetNamespace); err != nil {
			logger.Error(err, "Failed to update target Namespace labels")
			return ctrl.Result{}, err
		}
		logger.Info("Successfully updated labels", "namespace", targetNamespace.Name)
	}

	// update CR status
	return r.updateCRStatus(ctx, cr, skippedProtected)
}

func (r *NamespaceLabelReconciler) handleDelete(ctx context.Context, cr *namespacelabelv1alpha1.NamespaceLabel, protectedPrefixes []string) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Starting Delete CR event of the namespace " + cr.Namespace)

	if controllerutil.ContainsFinalizer(cr, namespaceLabelFinalizer) {
		logger.Info("Cleanup triggered, removing managed labels")

		var targetNamespace corev1.Namespace
		if err := r.Get(ctx, types.NamespacedName{Name: cr.Namespace}, &targetNamespace); err == nil {

			// Passing 'nil' for spec labels forces the helper to delete ALL managed keys
			needsUpdate := r.removeStaleLabels(ctx, &targetNamespace, protectedPrefixes, nil)

			// cleanup annotation
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
		controllerutil.RemoveFinalizer(cr, namespaceLabelFinalizer)
		if err := r.Update(ctx, cr); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// helper methods :

func (r *NamespaceLabelReconciler) getProtectedPrefixes(ctx context.Context) []string {
	logger := logf.FromContext(ctx)
	var policyConfig corev1.ConfigMap

	err := r.Get(ctx, types.NamespacedName{Name: protectedConfigMapName, Namespace: protectedConfigLocation}, &policyConfig)
	if err == nil {
		prefixesString := policyConfig.Data["protected-prefixes"]

		// guard against empty strings or missing keys
		if strings.TrimSpace(prefixesString) != "" {
			return strings.Split(prefixesString, ",")
		}
		logger.Info("ConfigMap found, but 'protected-prefixes' key is empty or missing. using default protected labels")
	} else {
		logger.Info("Policy ConfigMap not found, using default protected labels")
	}

	return strings.Split(defaultProtectedPrefixesStr, ",")
}

func (r *NamespaceLabelReconciler) isProtected(key string, protectedPrefixes []string) bool {
	for _, prefix := range protectedPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// deletes unwanted labels
// If currentSpecLabels is nil, it assumes a full cleanup (Delete event)
func (r *NamespaceLabelReconciler) removeStaleLabels(ctx context.Context, ns *corev1.Namespace, prefixes []string, currentSpecLabels map[string]string) bool {
	logger := logf.FromContext(ctx)
	needsUpdate := false

	oldKeysString := ns.Annotations[managedKeysAnnotation]
	if oldKeysString == "" {
		return false
	}

	oldKeys := strings.Split(oldKeysString, ",")
	for _, oldKey := range oldKeys {
		_, existsInSpec := currentSpecLabels[oldKey]

		// If it's a full cleanup (currentSpecLabels is nil) OR the key was removed from the CR spec
		if currentSpecLabels == nil || !existsInSpec {
			if r.isProtected(oldKey, prefixes) {
				logger.Info("Cannot delete label; it is protected", "key", oldKey)
				continue
			}

			// checking if it exists
			if _, hasLabel := ns.Labels[oldKey]; hasLabel {
				logger.Info("Removing label", "key", oldKey)
				delete(ns.Labels, oldKey)
				needsUpdate = true
			}
		}
	}
	return needsUpdate
}

func (r *NamespaceLabelReconciler) applyNewLabels(ctx context.Context, ns *corev1.Namespace, prefixes []string, currentSpecLabels map[string]string) (bool, []string, bool) {
	logger := logf.FromContext(ctx)
	needsUpdate := false
	skippedProtected := false
	var currentKeys []string

	for key, value := range currentSpecLabels {
		// dont let protected keys to get mannaged by the operator
		if r.isProtected(key, prefixes) {
			logger.Info("Skipping protected label in CR", "key", key)
			skippedProtected = true
			continue
		}

		// only trigger update if the label doesn't exist or the value is different
		if ns.Labels[key] != value {
			ns.Labels[key] = value
			needsUpdate = true
		}
		currentKeys = append(currentKeys, key)
	}

	return needsUpdate, currentKeys, skippedProtected
}

func (r *NamespaceLabelReconciler) updateCRStatus(ctx context.Context, cr *namespacelabelv1alpha1.NamespaceLabel, skippedProtected bool) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	newApplied := true
	newMessage := "Successfully synced all labels"
	if skippedProtected {
		newMessage = "Applied labels, but skipped protected keys"
	}

	// only make the API call if the status actually needs to change
	if cr.Status.Applied != newApplied || cr.Status.Message != newMessage {
		cr.Status.Applied = newApplied
		cr.Status.Message = newMessage

		if err := r.Status().Update(ctx, cr); err != nil {
			logger.Error(err, "Failed to update CR status")
			return ctrl.Result{}, err
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
			handler.EnqueueRequestsFromMapFunc(r.getCustomResourcesInNamespace),
			builder.WithPredicates(r.namespaceLabelDriftPredicate()),
		).
		Complete(r)
}

func (r *NamespaceLabelReconciler) namespaceLabelDriftPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldNs, okOld := e.ObjectOld.(*corev1.Namespace)
			newNs, okNew := e.ObjectNew.(*corev1.Namespace)
			// if the change is in a namespace (if the fetch succeeded)
			if !okOld || !okNew {
				return false
			}

			// check if there are any managed labels
			managedKeysStr := newNs.GetAnnotations()[managedKeysAnnotation]
			if managedKeysStr == "" {
				return false
			}

			keys := strings.Split(managedKeysStr, ",")
			// checks diffs between the namespace's labels

			for _, key := range keys {
				if oldNs.GetLabels()[key] != newNs.GetLabels()[key] {
					return true
				}
			}
			return false
		},
		// return false for other namespace changes
		CreateFunc:  func(e event.CreateEvent) bool { return false },
		DeleteFunc:  func(e event.DeleteEvent) bool { return false },
		GenericFunc: func(e event.GenericEvent) bool { return false },
	}
}

func (r *NamespaceLabelReconciler) getCustomResourcesInNamespace(ctx context.Context, obj client.Object) []reconcile.Request {
	ns, ok := obj.(*corev1.Namespace)
	// is casting the event into namespace succeeded
	if !ok {
		return nil
	}

	var crList namespacelabelv1alpha1.NamespaceLabelList
	// List ALL NamespaceLabel CRs in this namespace, drop if there is none \ theres an error
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
