package controllers_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	redisv1alpha1 "github.com/example/redis-operator/api/v1alpha1"
)

const (
	timeout  = 10 * time.Second
	interval = 250 * time.Millisecond
)

func newRedisInstance(name, namespace string) *redisv1alpha1.RedisInstance {
	return &redisv1alpha1.RedisInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: redisv1alpha1.RedisInstanceSpec{
			Replicas:     1,
			RedisVersion: "redis:7.2",
			Topology:     redisv1alpha1.TopologyStandalone,
			Storage: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("1Gi"),
					},
				},
			},
		},
	}
}

var _ = Describe("RedisInstance controller", func() {
	const namespace = "default"

	Context("When creating a RedisInstance", func() {
		It("Should create owned resources and add finalizer", func() {
			ctx := context.Background()
			instance := newRedisInstance("test-redis", namespace)
			Expect(k8sClient.Create(ctx, instance)).To(Succeed())

			instanceKey := types.NamespacedName{Name: "test-redis", Namespace: namespace}

			By("Adding the finalizer")
			Eventually(func() bool {
				ri := &redisv1alpha1.RedisInstance{}
				if err := k8sClient.Get(ctx, instanceKey, ri); err != nil {
					return false
				}
				for _, f := range ri.Finalizers {
					if f == redisv1alpha1.FinalizerName {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			By("Creating a ConfigMap")
			Eventually(func() error {
				cm := &corev1.ConfigMap{}
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "test-redis-config",
					Namespace: namespace,
				}, cm)
			}, timeout, interval).Should(Succeed())

			By("Creating a headless Service")
			Eventually(func() bool {
				svc := &corev1.Service{}
				if err := k8sClient.Get(ctx, instanceKey, svc); err != nil {
					return false
				}
				return svc.Spec.ClusterIP == "None"
			}, timeout, interval).Should(BeTrue())

			By("Creating a StatefulSet")
			Eventually(func() error {
				sts := &appsv1.StatefulSet{}
				return k8sClient.Get(ctx, instanceKey, sts)
			}, timeout, interval).Should(Succeed())

			// Cleanup
			Expect(k8sClient.Delete(ctx, instance)).To(Succeed())
		})
	})

	Context("When updating a RedisInstance", func() {
		It("Should update StatefulSet replica count on spec change", func() {
			ctx := context.Background()
			instance := newRedisInstance("test-redis-update", namespace)
			Expect(k8sClient.Create(ctx, instance)).To(Succeed())

			instanceKey := types.NamespacedName{Name: "test-redis-update", Namespace: namespace}

			By("Waiting for StatefulSet to be created")
			Eventually(func() error {
				sts := &appsv1.StatefulSet{}
				return k8sClient.Get(ctx, instanceKey, sts)
			}, timeout, interval).Should(Succeed())

			By("Scaling up to 3 replicas")
			ri := &redisv1alpha1.RedisInstance{}
			Expect(k8sClient.Get(ctx, instanceKey, ri)).To(Succeed())
			ri.Spec.Replicas = 3
			Expect(k8sClient.Update(ctx, ri)).To(Succeed())

			By("StatefulSet should have 3 replicas")
			Eventually(func() int32 {
				sts := &appsv1.StatefulSet{}
				if err := k8sClient.Get(ctx, instanceKey, sts); err != nil {
					return 0
				}
				if sts.Spec.Replicas == nil {
					return 0
				}
				return *sts.Spec.Replicas
			}, timeout, interval).Should(Equal(int32(3)))

			// Cleanup
			Expect(k8sClient.Delete(ctx, instance)).To(Succeed())
		})
	})

	Context("When deleting a RedisInstance", func() {
		It("Should remove finalizer and allow deletion", func() {
			ctx := context.Background()
			instance := newRedisInstance("test-redis-delete", namespace)
			Expect(k8sClient.Create(ctx, instance)).To(Succeed())

			instanceKey := types.NamespacedName{Name: "test-redis-delete", Namespace: namespace}

			By("Waiting for finalizer to be added")
			Eventually(func() bool {
				ri := &redisv1alpha1.RedisInstance{}
				if err := k8sClient.Get(ctx, instanceKey, ri); err != nil {
					return false
				}
				for _, f := range ri.Finalizers {
					if f == redisv1alpha1.FinalizerName {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			By("Deleting the instance")
			ri := &redisv1alpha1.RedisInstance{}
			Expect(k8sClient.Get(ctx, instanceKey, ri)).To(Succeed())
			Expect(k8sClient.Delete(ctx, ri)).To(Succeed())

			By("Instance should be eventually deleted")
			Eventually(func() bool {
				ri := &redisv1alpha1.RedisInstance{}
				err := k8sClient.Get(ctx, instanceKey, ri)
				return err != nil
			}, timeout, interval).Should(BeTrue())
		})
	})
})
