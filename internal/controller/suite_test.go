package controller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	redisv1alpha1 "github.com/example/redis-operator/api/v1alpha1"
)

var (
	testEnv    *envtest.Environment
	cfg        *rest.Config
	k8sClient  client.Client
	testScheme = k8sruntime.NewScheme()
)

func TestMain(m *testing.M) {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	_ = scheme.AddToScheme(testScheme)
	_ = appsv1.AddToScheme(testScheme)
	_ = corev1.AddToScheme(testScheme)
	_ = redisv1alpha1.AddToScheme(testScheme)

	_, currentFile, _, _ := goruntime.Caller(0)
	crdPath := filepath.Join(filepath.Dir(currentFile), "..", "..", "config", "crd", "bases")
	testEnv = &envtest.Environment{CRDDirectoryPaths: []string{crdPath}}

	var err error
	cfg, err = testEnv.Start()
	if err != nil {
		panic(fmt.Sprintf("starting envtest: %v", err))
	}

	k8sClient, err = client.New(cfg, client.Options{Scheme: testScheme})
	if err != nil {
		panic(fmt.Sprintf("creating client: %v", err))
	}

	code := m.Run()

	if err := testEnv.Stop(); err != nil {
		panic(fmt.Sprintf("stopping envtest: %v", err))
	}
	os.Exit(code)
}

func newTestReconciler(t *testing.T) *RedisInstanceReconciler {
	t.Helper()

	c, err := client.New(cfg, client.Options{Scheme: testScheme})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	return &RedisInstanceReconciler{
		Client:   c,
		Scheme:   testScheme,
		Recorder: record.NewFakeRecorder(64),
	}
}

func runReconcile(t *testing.T, r *RedisInstanceReconciler, namespace, name string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKey{Namespace: namespace, Name: name}}); err != nil {
		t.Fatalf("reconcile %s/%s: %v", namespace, name, err)
	}
}
