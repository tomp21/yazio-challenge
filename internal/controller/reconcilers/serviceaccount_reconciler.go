package reconcilers

import (
	"context"
	cachev1alpha1 "github.com/tomp21/yazio-challenge/api/v1alpha1"
	"github.com/tomp21/yazio-challenge/internal/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type ServiceAccountReconciler struct {
	client *client.Client
	scheme *runtime.Scheme
}

func NewServiceAccountReconciler(client *client.Client, scheme *runtime.Scheme) *ServiceAccountReconciler {
	return &ServiceAccountReconciler{
		client: client,
	}
}

func (r *ServiceAccountReconciler) Reconcile(ctx context.Context, redis *cachev1alpha1.Redis) error {

	return nil
}

func (r *ServiceAccountReconciler) createOrUpdateServiceAccount(ctx context.Context, redis *cachev1alpha1.Redis, scheme *runtime.Scheme) error {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redis.Name,
			Namespace: redis.Namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, *r.client, sa, func() error {
		sa.GetObjectMeta().SetLabels(util.GetLabels(redis, nil))
		return controllerutil.SetControllerReference(redis, sa, scheme)
	})
	return err
}
