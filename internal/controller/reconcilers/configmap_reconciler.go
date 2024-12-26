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
	return nil
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
	if err != nil {
		return err
	}
	return nil
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

	if err != nil {
		return err
	}

	return nil
}
