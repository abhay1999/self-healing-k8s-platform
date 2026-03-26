package main

import (
	"context"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// --- helpers ---

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	return s
}

// makePod creates a Pod with one ContainerStatus per restart count provided.
func makePod(name, namespace string, restartCounts ...int32) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	for i, count := range restartCounts {
		pod.Status.ContainerStatuses = append(pod.Status.ContainerStatuses, corev1.ContainerStatus{
			Name:         fmt.Sprintf("container-%d", i),
			RestartCount: count,
		})
	}
	return pod
}

func newReconciler(t *testing.T, objs ...client.Object) *PodReconciler {
	t.Helper()
	s := newScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(&corev1.Pod{}).
		Build()
	return &PodReconciler{Client: c, Scheme: s}
}

func reconcileFor(t *testing.T, r *PodReconciler, name, namespace string) (ctrl.Result, error) {
	t.Helper()
	return r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: name, Namespace: namespace},
	})
}

func podExists(t *testing.T, r *PodReconciler, name, namespace string) bool {
	t.Helper()
	var pod corev1.Pod
	err := r.Get(context.Background(), types.NamespacedName{Name: name, Namespace: namespace}, &pod)
	if err == nil {
		return true
	}
	if client.IgnoreNotFound(err) == nil {
		return false // NotFound → deleted
	}
	t.Fatalf("unexpected error checking pod existence: %v", err)
	return false
}

// --- tests: pod not found ---

func TestReconcile_PodNotFound_NoError(t *testing.T) {
	r := newReconciler(t) // empty store — pod doesn't exist

	_, err := reconcileFor(t, r, "missing", "default")

	if err != nil {
		t.Errorf("want no error for missing pod, got %v", err)
	}
}

// --- tests: system namespace skips ---

func TestReconcile_SkipsKubeSystem(t *testing.T) {
	pod := makePod("sys-pod", "kube-system", 99)
	r := newReconciler(t, pod)

	_, err := reconcileFor(t, r, "sys-pod", "kube-system")

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !podExists(t, r, "sys-pod", "kube-system") {
		t.Error("pod in kube-system should not be deleted regardless of restart count")
	}
}

func TestReconcile_SkipsMonitoring(t *testing.T) {
	pod := makePod("prom-pod", "monitoring", 99)
	r := newReconciler(t, pod)

	_, err := reconcileFor(t, r, "prom-pod", "monitoring")

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !podExists(t, r, "prom-pod", "monitoring") {
		t.Error("pod in monitoring namespace should not be deleted")
	}
}

// --- tests: restart threshold ---

func TestReconcile_BelowThreshold_PodKept(t *testing.T) {
	pod := makePod("ok-pod", "default", 2) // 2 < maxRestarts(3)
	r := newReconciler(t, pod)

	_, err := reconcileFor(t, r, "ok-pod", "default")

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !podExists(t, r, "ok-pod", "default") {
		t.Error("pod below restart threshold should not be deleted")
	}
}

func TestReconcile_AtThreshold_PodKept(t *testing.T) {
	// The condition is `> maxRestarts`, so exactly 3 restarts should NOT trigger deletion.
	pod := makePod("edge-pod", "default", int32(maxRestarts))
	r := newReconciler(t, pod)

	_, err := reconcileFor(t, r, "edge-pod", "default")

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !podExists(t, r, "edge-pod", "default") {
		t.Errorf("pod with exactly %d restarts (not >) should not be deleted", maxRestarts)
	}
}

func TestReconcile_AboveThreshold_PodDeleted(t *testing.T) {
	pod := makePod("crashy-pod", "default", int32(maxRestarts+1)) // 4 > 3
	r := newReconciler(t, pod)

	_, err := reconcileFor(t, r, "crashy-pod", "default")

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if podExists(t, r, "crashy-pod", "default") {
		t.Error("pod above restart threshold should have been deleted")
	}
}

func TestReconcile_WellAboveThreshold_PodDeleted(t *testing.T) {
	pod := makePod("looping-pod", "default", 50)
	r := newReconciler(t, pod)

	_, err := reconcileFor(t, r, "looping-pod", "default")

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if podExists(t, r, "looping-pod", "default") {
		t.Error("pod with 50 restarts should have been deleted")
	}
}

// --- tests: multi-container pods ---

func TestReconcile_MultiContainer_AllBelowThreshold(t *testing.T) {
	pod := makePod("multi-ok", "default", 1, 2) // both below maxRestarts
	r := newReconciler(t, pod)

	_, err := reconcileFor(t, r, "multi-ok", "default")

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !podExists(t, r, "multi-ok", "default") {
		t.Error("pod with all containers below threshold should not be deleted")
	}
}

func TestReconcile_MultiContainer_OneAboveThreshold(t *testing.T) {
	pod := makePod("multi-crash", "default", 1, int32(maxRestarts+1)) // second container crashes
	r := newReconciler(t, pod)

	_, err := reconcileFor(t, r, "multi-crash", "default")

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if podExists(t, r, "multi-crash", "default") {
		t.Error("pod should be deleted when any container exceeds restart threshold")
	}
}

func TestReconcile_NilContainerStatuses_NoError(t *testing.T) {
	pod := makePod("no-status-pod", "default") // no container statuses at all
	r := newReconciler(t, pod)

	_, err := reconcileFor(t, r, "no-status-pod", "default")

	if err != nil {
		t.Errorf("pod with empty ContainerStatuses should not cause an error: %v", err)
	}
	if !podExists(t, r, "no-status-pod", "default") {
		t.Error("pod with no container statuses should not be deleted")
	}
}
