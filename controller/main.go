// Self-Healing Kubernetes Controller
//
// This controller watches all Pods in the cluster. When a pod has restarted
// more than `maxRestarts` times, it deletes the pod — forcing Kubernetes to
// schedule a fresh replacement. All actions are logged.
//
// Week 4 — Day 22–24
package main

import (
	"context"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const maxRestarts = 3

// PodReconciler watches Pod objects and auto-heals them.
type PodReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile is called every time a Pod is created, updated, or deleted.
func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the Pod
	var pod corev1.Pod
	if err := r.Get(ctx, req.NamespacedName, &pod); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Skip system namespaces
	if pod.Namespace == "kube-system" || pod.Namespace == "monitoring" {
		return ctrl.Result{}, nil
	}

	// Check each container's restart count
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.RestartCount > maxRestarts {
			logger.Info(fmt.Sprintf(
				"[SELF-HEAL] Pod %s/%s container %s has restarted %d times (threshold: %d) — deleting pod",
				pod.Namespace, pod.Name, cs.Name, cs.RestartCount, maxRestarts,
			))

			if err := r.Delete(ctx, &pod); err != nil {
				logger.Error(err, "Failed to delete pod", "pod", pod.Name)
				return ctrl.Result{}, err
			}

			logger.Info(fmt.Sprintf("[SELF-HEAL] Deleted pod %s/%s — K8s will schedule a fresh replacement", pod.Namespace, pod.Name))
			return ctrl.Result{}, nil
		}
	}

	return ctrl.Result{}, nil
}

func main() {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	logger := ctrl.Log.WithName("self-healing-controller")

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
	})
	if err != nil {
		logger.Error(err, "Failed to create manager")
		os.Exit(1)
	}

	if err := (&PodReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		logger.Error(err, "Failed to set up controller")
		os.Exit(1)
	}

	logger.Info("Starting self-healing controller", "maxRestarts", maxRestarts)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		logger.Error(err, "Manager exited with error")
		os.Exit(1)
	}
}

// SetupWithManager registers this reconciler to watch Pod events.
func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		Complete(r)
}
