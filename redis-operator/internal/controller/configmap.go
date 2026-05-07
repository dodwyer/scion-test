package controller

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	redisv1alpha1 "github.com/example/redis-operator/api/v1alpha1"
)

// reconcileConfigMap creates or updates the redis.conf ConfigMap.
func (r *RedisInstanceReconciler) reconcileConfigMap(ctx context.Context, ri *redisv1alpha1.RedisInstance) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ri.Name + "-config",
			Namespace: ri.Namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		cm.Labels = resourceLabels(ri.Name, componentRedis)
		cm.Data = map[string]string{
			"redis.conf": renderRedisConf(ri.Spec.Config),
		}
		return controllerutil.SetControllerReference(ri, cm, r.Scheme)
	})
	return err
}

// renderRedisConf converts a key/value map into a redis.conf formatted string.
// Keys are sorted for deterministic output (stable config hashes).
func renderRedisConf(config map[string]string) string {
	if len(config) == 0 {
		return "# redis.conf managed by redis-operator\n"
	}
	keys := make([]string, 0, len(config))
	for k := range config {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	fmt.Fprintln(&sb, "# redis.conf managed by redis-operator")
	for _, k := range keys {
		fmt.Fprintf(&sb, "%s %s\n", k, config[k])
	}
	return sb.String()
}
