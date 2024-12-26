package reconcilers

import (
	_ "embed"

	"context"
	cachev1alpha1 "github.com/tomp21/yazio-challenge/api/v1alpha1"
	"github.com/tomp21/yazio-challenge/internal/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

//go:embed configmaps/scripts/start-master.sh
var masterStartScript string

//go:embed configmaps/scripts/start-replica.sh
var replicaStartScript string

//go:embed configmaps/conf/redis.conf
var redisAllConf string

//go:embed configmaps/conf/master.conf
var redisMasterConf string

//go:embed configmaps/conf/replica.conf
var redisReplicaConf string

//go:embed configmaps/health/ping_liveness_local.sh
var redisLivenessLocal string

//go:embed configmaps/health/ping_readiness_local.sh
var redisReadinessLocal string

//go:embed configmaps/health/ping_liveness_master.sh
var redisLivenessMaster string

//go:embed configmaps/health/ping_readiness_master.sh
var redisReadinessMaster string

//go:embed configmaps/health/ping_liveness_local_and_master.sh
var redisLivenessLocalMaster string

//go:embed configmaps/health/ping_readiness_local_and_master.sh
var redisReadinessLocalMaster string

type ConfigMapReconciler struct {
	client *client.Client
	scheme *runtime.Scheme
}

func NewConfigMapReconciler(client *client.Client, scheme *runtime.Scheme) *ConfigMapReconciler {
	return &ConfigMapReconciler{
		client: client,
		scheme: scheme,
	}
}

func (r *ConfigMapReconciler) Reconcile(ctx context.Context, redis *cachev1alpha1.Redis) error {
	err := r.ReconcileStartScripts(ctx, redis)
	if err != nil {
		return err
	}
	err = r.ReconcileConfigs(ctx, redis)
	if err != nil {
		return err
	}

	err = r.ReconcileHealthScripts(ctx, redis)

	return err

}

func (r *ConfigMapReconciler) ReconcileStartScripts(ctx context.Context, redis *cachev1alpha1.Redis) error {
	cmStartScripts := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "start-scripts",
			Namespace: redis.Namespace,
			Labels:    util.GetLabels(redis, nil),
		},
		Data: map[string]string{
			"start-master.sh":  masterStartScript,
			"start-replica.sh": replicaStartScript,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, *r.client, cmStartScripts, func() error {
		return controllerutil.SetControllerReference(redis, cmStartScripts, r.scheme)
	})

	return err
}

func (r *ConfigMapReconciler) ReconcileConfigs(ctx context.Context, redis *cachev1alpha1.Redis) error {
	cmConfig := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redis-conf",
			Namespace: redis.Namespace,
			Labels:    util.GetLabels(redis, nil),
		},
		Data: map[string]string{
			"redis.conf":   redisAllConf,
			"master.conf":  redisMasterConf,
			"replica.conf": redisReplicaConf,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, *r.client, cmConfig, func() error {
		return controllerutil.SetControllerReference(redis, cmConfig, r.scheme)
	})
	return err
}

func (r *ConfigMapReconciler) ReconcileHealthScripts(ctx context.Context, redis *cachev1alpha1.Redis) error {
	cmHealth := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redis-health",
			Namespace: redis.Namespace,
			Labels:    util.GetLabels(redis, nil),
		},
		Data: map[string]string{
			"ping_liveness_local.sh":             redisLivenessLocal,
			"ping_readiness_local.sh":            redisReadinessLocal,
			"ping_liveness_master.sh":            redisLivenessMaster,
			"ping_readiness_master.sh":           redisReadinessMaster,
			"ping_liveness_local_and_master.sh":  redisLivenessLocalMaster,
			"ping_readiness_local_and_master.sh": redisReadinessLocalMaster,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, *r.client, cmHealth, func() error {
		return controllerutil.SetControllerReference(redis, cmHealth, r.scheme)
	})

	return err
}
