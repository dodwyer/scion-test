// Package integration contains envtest-based integration tests for the redis-operator.
// Run with: KUBEBUILDER_ASSETS=$(setup-envtest use -p path 1.31) go test ./test/integration/...
package integration

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	runtimescheme "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	redisv1alpha1 "github.com/example/redis-operator/api/v1alpha1"
	"github.com/example/redis-operator/internal/controller"
)

var (
	testEnv   *envtest.Environment
	k8sClient client.Client
	cancelFn  context.CancelFunc
)

func TestMain(m *testing.M) {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	scheme := runtimescheme.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(redisv1alpha1.AddToScheme(scheme))
	utilruntime.Must(appsv1.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))

	// CRD path relative to this file's location.
	_, thisFile, _, _ := runtime.Caller(0)
	crdPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "config", "crd", "bases")

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{crdPath},
		ErrorIfCRDPathMissing: true,
		Scheme:                scheme,
	}

	cfg, err := testEnv.Start()
	if err != nil {
		panic(err)
	}

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		panic(err)
	}

	// Start the controller manager.
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 scheme,
		LeaderElection:         false,
		HealthProbeBindAddress: "0",
		Metrics:                ctrl.Options{}.Metrics,
	})
	if err != nil {
		panic(err)
	}

	if err = (&controller.RedisInstanceReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("redis-operator"),
	}).SetupWithManager(mgr); err != nil {
		panic(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancelFn = cancel

	go func() {
		if err := mgr.Start(ctx); err != nil {
			panic(err)
		}
	}()

	// Wait for manager cache to sync.
	time.Sleep(500 * time.Millisecond)

	code := m.Run()

	cancel()
	if err := testEnv.Stop(); err != nil {
		_ = err // best-effort teardown
	}
	os.Exit(code)
}
