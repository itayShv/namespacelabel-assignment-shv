package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	namespacelabelv1alpha1 "github.com/itayShv/namespacelabel-assignment-shv/api/v1alpha1"
)

// NamespaceLabelReconciler reconciles a NamespaceLabel object
type NamespaceLabelReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=namespacelabel.dana.exam,resources=namespacelabels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=namespacelabel.dana.exam,resources=namespacelabels/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=namespacelabel.dana.exam,resources=namespacelabels/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NamespaceLabel object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.3/pkg/reconcile
func (r *NamespaceLabelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Starting reconciliation for the NameSpace Label Operator")
	
	var namespaceLabelCR namespacelabelv1alpha1.NamespaceLabel
	if err := r.Get(ctx, req.NamespacedName, &NamespaceLabel); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}


	// create - check protected lables that are found in the ConfigMap

	// update

	// returned none and the event was delete
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespaceLabelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&namespacelabelv1alpha1.NamespaceLabel{}).
		Named("namespacelabel").
		Complete(r)
}
