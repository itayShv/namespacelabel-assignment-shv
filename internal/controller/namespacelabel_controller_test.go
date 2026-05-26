package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	namespacelabelv1alpha1 "github.com/itayShv/namespacelabel-assignment-shv/api/v1alpha1"
)

var _ = Describe("NamespaceLabel Controller Testing", func() {

	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("Standard Lifecycle and Basic Edge Cases", func() {
		It("Should handle the complete lifecycle (CRUD, Singleton, Drift, Cleanup)", func() {
			ctx := context.Background()

			// SETUP: Create a real namespace with an unmanaged label
			nsName := "test-namespace"
			targetNamespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
					Labels: map[string]string{
						"unmanaged-label":             "do-not-touch",
						"kubernetes.io/metadata.name": "test-namespace", // protected label
					},
				},
			}
			Expect(k8sClient.Create(ctx, targetNamespace)).Should(Succeed())

			// Creating first time (no ConfigMap and Annotations)
			// AND validating protected prefixes are ignored on creation
			validCR := &namespacelabelv1alpha1.NamespaceLabel{
				ObjectMeta: metav1.ObjectMeta{Name: "labels", Namespace: nsName},
				Spec: namespacelabelv1alpha1.NamespaceLabelSpec{
					Labels: map[string]string{
						"team":                 "backend",
						"env":                  "dev",
						"kubernetes.io/hacked": "true", // false attempt to create protected label
					},
				},
			}
			// This represents CRUD: Create
			Expect(k8sClient.Create(ctx, validCR)).Should(Succeed())

			Eventually(func() map[string]string {
				var ns corev1.Namespace
				k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, &ns)
				return ns.Labels
			}, timeout, interval).Should(SatisfyAll(
				HaveKeyWithValue("team", "backend"),
				HaveKeyWithValue("env", "dev"),
				HaveKeyWithValue("unmanaged-label", "do-not-touch"),               // Left alone
				HaveKeyWithValue("kubernetes.io/metadata.name", "test-namespace"), // Left alone
				Not(HaveKey("kubernetes.io/hacked")),                              // Protected prefix creation BLOCKED
			))

			// Validate protected prefixes cannot be deleted
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "labels", Namespace: nsName}, validCR)).Should(Succeed())
			validCR.Spec.Labels = map[string]string{
				"team": "backend",
				"env":  "prod", // add this
			}
			Expect(k8sClient.Update(ctx, validCR)).Should(Succeed())

			Eventually(func() map[string]string {
				var ns corev1.Namespace
				k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, &ns)
				return ns.Labels
			}, timeout, interval).Should(SatisfyAll(
				HaveKeyWithValue("env", "prod"),
				HaveKeyWithValue("kubernetes.io/metadata.name", "test-namespace"), // Protected prefix deletion BLOCKED
			))

			// Drift change (Admin deletes a managed label)
			var currentNs corev1.Namespace
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, &currentNs)).Should(Succeed())

			// Simulate cluster admin manually deleting 'team'
			delete(currentNs.Labels, "team")
			Expect(k8sClient.Update(ctx, &currentNs)).Should(Succeed())

			// Validate that the Drift Predicate puts it back
			Eventually(func() map[string]string {
				var ns corev1.Namespace
				k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, &ns)
				return ns.Labels
			}, timeout, interval).Should(HaveKeyWithValue("team", "backend"))

			// Complete Cleanup CRUD: Delete
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "labels", Namespace: nsName}, validCR)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, validCR)).Should(Succeed())

			Eventually(func() map[string]string {
				var ns corev1.Namespace
				k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, &ns)
				return ns.Labels
			}, timeout, interval).Should(SatisfyAll(
				Not(HaveKey("env")),  // Managed label cleaned up
				Not(HaveKey("team")), // Managed label cleaned up
				HaveKeyWithValue("unmanaged-label", "do-not-touch"),               // UNMANAGED LABEL SURVIVED
				HaveKeyWithValue("kubernetes.io/metadata.name", "test-namespace"), // PROTECTED LABEL SURVIVED
			))

			// Verify the tracking annotation was completely removed
			Eventually(func() map[string]string {
				var ns corev1.Namespace
				k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, &ns)
				return ns.Annotations
			}, timeout, interval).ShouldNot(HaveKey(managedKeysAnnotation))
		})
	})

	Context("Advanced Edge Cases", func() {
		It("Should completely wipe managed labels if the CR Spec is emptied", func() {
			ctx := context.Background()
			nsName := "test-empty-wipe"
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())

			cr := &namespacelabelv1alpha1.NamespaceLabel{
				ObjectMeta: metav1.ObjectMeta{Name: "labels", Namespace: nsName},
				Spec: namespacelabelv1alpha1.NamespaceLabelSpec{
					Labels: map[string]string{"temp-key": "temp-value"},
				},
			}
			Expect(k8sClient.Create(ctx, cr)).Should(Succeed())

			// Wait for the label to appear
			Eventually(func() map[string]string {
				var fetchedNs corev1.Namespace
				k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, &fetchedNs)
				return fetchedNs.Labels
			}, timeout, interval).Should(HaveKeyWithValue("temp-key", "temp-value"))

			// User updates the CR to have NO labels
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "labels", Namespace: nsName}, cr)).Should(Succeed())
			cr.Spec.Labels = map[string]string{} // Empty map
			Expect(k8sClient.Update(ctx, cr)).Should(Succeed())

			// Verify the label was wiped from the Namespace
			Eventually(func() map[string]string {
				var fetchedNs corev1.Namespace
				k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, &fetchedNs)
				return fetchedNs.Labels
			}, timeout, interval).ShouldNot(HaveKey("temp-key"))
		})

		It("Should correctly update the Status subresource", func() {
			ctx := context.Background()
			nsName := "test-status-update"
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())

			cr := &namespacelabelv1alpha1.NamespaceLabel{
				ObjectMeta: metav1.ObjectMeta{Name: "labels", Namespace: nsName},
				Spec: namespacelabelv1alpha1.NamespaceLabelSpec{
					Labels: map[string]string{
						"valid":            "ok",
						"kubernetes.io/no": "blocked", // Protected
					},
				},
			}
			Expect(k8sClient.Create(ctx, cr)).Should(Succeed())

			// Verify the Status fields are populated correctly indicating a partial success
			Eventually(func() string {
				var fetchedCR namespacelabelv1alpha1.NamespaceLabel
				k8sClient.Get(ctx, types.NamespacedName{Name: "labels", Namespace: nsName}, &fetchedCR)
				return fetchedCR.Status.Message
			}, timeout, interval).Should(Equal("Applied labels, but skipped protected keys"))

			Eventually(func() bool {
				var fetchedCR namespacelabelv1alpha1.NamespaceLabel
				k8sClient.Get(ctx, types.NamespacedName{Name: "labels", Namespace: nsName}, &fetchedCR)
				return fetchedCR.Status.Applied
			}, timeout, interval).Should(BeTrue())
		})

		It("Should handle stale annotation cleanup (Self-Healing)", func() {
			ctx := context.Background()
			nsName := "test-stale-cleanup"
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())

			cr := &namespacelabelv1alpha1.NamespaceLabel{
				ObjectMeta: metav1.ObjectMeta{Name: "labels", Namespace: nsName},
				Spec: namespacelabelv1alpha1.NamespaceLabelSpec{
					Labels: map[string]string{"valid-key": "valid-value"},
				},
			}
			Expect(k8sClient.Create(ctx, cr)).Should(Succeed())

			// Wait for initial sync
			Eventually(func() map[string]string {
				var fetchedNs corev1.Namespace
				k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, &fetchedNs)
				return fetchedNs.Labels
			}, timeout, interval).Should(HaveKeyWithValue("valid-key", "valid-value"))

			// Simulate stale state: Manually add a rogue label and inject it into the managed annotation
			var currentNs corev1.Namespace
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, &currentNs)).Should(Succeed())

			if currentNs.Labels == nil {
				currentNs.Labels = make(map[string]string)
			}
			currentNs.Labels["stale-key"] = "stale-value"
			currentNs.Annotations[managedKeysAnnotation] = "valid-key,stale-key"
			Expect(k8sClient.Update(ctx, &currentNs)).Should(Succeed())

			// Trigger Reconcile via CR update to force logic evaluation
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "labels", Namespace: nsName}, cr)).Should(Succeed())
			cr.Spec.Labels["valid-key"] = "new-value"
			Expect(k8sClient.Update(ctx, cr)).Should(Succeed())

			// Verify stale-key is successfully removed because it is not in the CR's Spec
			Eventually(func() map[string]string {
				var fetchedNs corev1.Namespace
				k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, &fetchedNs)
				return fetchedNs.Labels
			}, timeout, interval).Should(SatisfyAll(
				HaveKeyWithValue("valid-key", "new-value"),
				Not(HaveKey("stale-key")),
			))
		})

		It("Should allow CR deletion even if the target Namespace is missing", func() {
			ctx := context.Background()
			nsName := "test-missing-ns"
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())

			cr := &namespacelabelv1alpha1.NamespaceLabel{
				ObjectMeta: metav1.ObjectMeta{Name: "labels", Namespace: nsName},
				Spec: namespacelabelv1alpha1.NamespaceLabelSpec{
					Labels: map[string]string{"foo": "bar"},
				},
			}
			Expect(k8sClient.Create(ctx, cr)).Should(Succeed())

			// Wait for the Finalizer to be applied by the controller
			Eventually(func() []string {
				var fetchedCR namespacelabelv1alpha1.NamespaceLabel
				k8sClient.Get(ctx, types.NamespacedName{Name: "labels", Namespace: nsName}, &fetchedCR)
				return fetchedCR.Finalizers
			}, timeout, interval).Should(ContainElement(namespaceLabelFinalizer))

			// Delete the target Namespace FIRST
			Expect(k8sClient.Delete(ctx, ns)).Should(Succeed())

			// Now attempt to delete the CR
			Expect(k8sClient.Delete(ctx, cr)).Should(Succeed())

			// Verify CR is actually gone (Controller should gracefully ignore the missing NS and remove the finalizer)
			Eventually(func() error {
				var fetchedCR namespacelabelv1alpha1.NamespaceLabel
				return k8sClient.Get(ctx, types.NamespacedName{Name: "labels", Namespace: nsName}, &fetchedCR)
			}, timeout, interval).Should(SatisfyAll(
				HaveOccurred(),
				WithTransform(client.IgnoreNotFound, BeNil()), // Ensure the error is specifically "Not Found"
			))
		})

		It("Should respect the dynamically updated protected-labels ConfigMap", func() {
			ctx := context.Background()
			nsName := "test-cm-update"
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())

			// Create custom ConfigMap
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: protectedConfigMapName, Namespace: protectedConfigLocation},
				Data:       map[string]string{"protected-prefixes": "kubernetes.io/,k8s.io/,shviros.label/"},
			}

			// Ensure clean state in EnvTest for the default namespace
			_ = k8sClient.Delete(ctx, cm)
			Expect(k8sClient.Create(ctx, cm)).Should(Succeed())

			cr := &namespacelabelv1alpha1.NamespaceLabel{
				ObjectMeta: metav1.ObjectMeta{Name: "labels", Namespace: nsName},
				Spec: namespacelabelv1alpha1.NamespaceLabelSpec{
					Labels: map[string]string{
						"shviros.label/tenant": "123",    // with a protected prefix
						"app":                  "my-app", // Allowed
					},
				},
			}
			Expect(k8sClient.Create(ctx, cr)).Should(Succeed())

			// Verify the custom prefix in the ConfigMap blocks the creation of 'shviros.label'
			Eventually(func() map[string]string {
				var fetchedNs corev1.Namespace
				k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, &fetchedNs)
				return fetchedNs.Labels
			}, timeout, interval).Should(SatisfyAll(
				HaveKeyWithValue("app", "my-app"),
				Not(HaveKey("shviros.label/tenant")),
			))

			Expect(k8sClient.Delete(ctx, cm)).Should(Succeed())
		})
	})
})
