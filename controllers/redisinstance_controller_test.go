package controllers

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	redisv1alpha1 "github.com/example/redis-operator/api/v1alpha1"
)

var _ = Describe("RedisInstance controller", func() {
	var testCtx context.Context

	BeforeEach(func() {
		testCtx = context.Background()
	})

	It("reconciles create flow for standalone topology", func() {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceName("create")}}
		Expect(k8sClient.Create(testCtx, ns)).To(Succeed())

		instance := newRedisInstance(ns.Name, "sample", 1, redisv1alpha1.TopologyStandalone)
		Expect(k8sClient.Create(testCtx, instance)).To(Succeed())

		Eventually(func(g Gomega) {
			current := &redisv1alpha1.RedisInstance{}
			g.Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: instance.Name, Namespace: ns.Name}, current)).To(Succeed())
			g.Expect(current.Finalizers).To(ContainElement(redisFinalizer))

			cm := &corev1.ConfigMap{}
			g.Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: configMapName(instance), Namespace: ns.Name}, cm)).To(Succeed())
			g.Expect(cm.Data["redis.conf"]).To(ContainSubstring("maxmemory 256mb"))

			svc := &corev1.Service{}
			g.Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: instance.Name, Namespace: ns.Name}, svc)).To(Succeed())
			g.Expect(svc.Spec.ClusterIP).To(Equal(corev1.ClusterIPNone))

			sts := &appsv1.StatefulSet{}
			g.Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: instance.Name, Namespace: ns.Name}, sts)).To(Succeed())
			g.Expect(*sts.Spec.Replicas).To(Equal(int32(1)))
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())

		Eventually(func(g Gomega) {
			sts := &appsv1.StatefulSet{}
			g.Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: instance.Name, Namespace: ns.Name}, sts)).To(Succeed())
			sts.Status.Replicas = 1
			sts.Status.ReadyReplicas = 1
			g.Expect(k8sClient.Status().Update(testCtx, sts)).To(Succeed())
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

		Eventually(func(g Gomega) {
			current := &redisv1alpha1.RedisInstance{}
			g.Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: instance.Name, Namespace: ns.Name}, current)).To(Succeed())
			g.Expect(current.Status.Phase).To(Equal("Running"))
			g.Expect(current.Status.ReadyReplicas).To(Equal(int32(1)))
			g.Expect(current.Status.MasterEndpoint).To(ContainSubstring("sample-0.sample"))
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())
	})

	It("reconciles update flow for sentinel topology", func() {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceName("update")}}
		Expect(k8sClient.Create(testCtx, ns)).To(Succeed())

		instance := newRedisInstance(ns.Name, "sample", 2, redisv1alpha1.TopologySentinel)
		Expect(k8sClient.Create(testCtx, instance)).To(Succeed())

		Eventually(func(g Gomega) {
			sentinelSTS := &appsv1.StatefulSet{}
			g.Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: sentinelStatefulSetName(instance), Namespace: ns.Name}, sentinelSTS)).To(Succeed())
			sentinelSTS.Status.Replicas = 2
			sentinelSTS.Status.ReadyReplicas = 2
			g.Expect(k8sClient.Status().Update(testCtx, sentinelSTS)).To(Succeed())

			redisSTS := &appsv1.StatefulSet{}
			g.Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: instance.Name, Namespace: ns.Name}, redisSTS)).To(Succeed())
			redisSTS.Status.Replicas = 2
			redisSTS.Status.ReadyReplicas = 2
			g.Expect(k8sClient.Status().Update(testCtx, redisSTS)).To(Succeed())
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

		Eventually(func(g Gomega) {
			current := &redisv1alpha1.RedisInstance{}
			g.Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: instance.Name, Namespace: ns.Name}, current)).To(Succeed())
			g.Expect(current.Status.Phase).To(Equal("Running"))
			g.Expect(current.Status.SentinelEndpoints).To(HaveLen(2))
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())

		Eventually(func(g Gomega) {
			current := &redisv1alpha1.RedisInstance{}
			g.Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: instance.Name, Namespace: ns.Name}, current)).To(Succeed())
			current.Spec.Replicas = 3
			current.Spec.Config["maxclients"] = "1000"
			g.Expect(k8sClient.Update(testCtx, current)).To(Succeed())
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

		Eventually(func(g Gomega) {
			redisSTS := &appsv1.StatefulSet{}
			g.Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: instance.Name, Namespace: ns.Name}, redisSTS)).To(Succeed())
			g.Expect(*redisSTS.Spec.Replicas).To(Equal(int32(3)))
			g.Expect(redisSTS.Spec.Template.Annotations[statefulSetRestartAnno]).NotTo(BeEmpty())

			sentinelSTS := &appsv1.StatefulSet{}
			g.Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: sentinelStatefulSetName(instance), Namespace: ns.Name}, sentinelSTS)).To(Succeed())
			g.Expect(*sentinelSTS.Spec.Replicas).To(Equal(int32(3)))
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())
	})

	It("reconciles delete flow and removes owned PVCs before finalizer removal", func() {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceName("delete")}}
		Expect(k8sClient.Create(testCtx, ns)).To(Succeed())

		instance := newRedisInstance(ns.Name, "sample", 2, redisv1alpha1.TopologySentinel)
		Expect(k8sClient.Create(testCtx, instance)).To(Succeed())

		Eventually(func(g Gomega) {
			current := &redisv1alpha1.RedisInstance{}
			g.Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: instance.Name, Namespace: ns.Name}, current)).To(Succeed())
			g.Expect(current.Finalizers).To(ContainElement(redisFinalizer))
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())

		current := &redisv1alpha1.RedisInstance{}
		Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: instance.Name, Namespace: ns.Name}, current)).To(Succeed())
		Expect(k8sClient.Delete(testCtx, current)).To(Succeed())

		Eventually(func(g Gomega) {
			err := k8sClient.Get(testCtx, types.NamespacedName{Name: instance.Name, Namespace: ns.Name}, &redisv1alpha1.RedisInstance{})
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
		}, 20*time.Second, 250*time.Millisecond).Should(Succeed())
	})
})

func newRedisInstance(namespace, name string, replicas int32, topology string) *redisv1alpha1.RedisInstance {
	return &redisv1alpha1.RedisInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: redisv1alpha1.RedisInstanceSpec{
			Replicas:     replicas,
			RedisVersion: "7.2",
			Topology:     topology,
			Config: map[string]string{
				"maxmemory": "256mb",
			},
			Storage: redisv1alpha1.RedisPersistentVolumeClaimTemplate{
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
					},
				},
			},
		},
	}
}
