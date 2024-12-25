package reconcilers

import (
	"context"
	"fmt"
	cachev1alpha1 "github.com/tomp21/yazio-challenge/api/v1alpha1"
	"github.com/tomp21/yazio-challenge/internal/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	RedisPort     = 6379
	RedisPortName = "redis"
)

type ServiceReconciler struct {
	client *client.Client
	scheme *runtime.Scheme
}

func NewServiceReconciler(client *client.Client, scheme *runtime.Scheme) *ServiceReconciler {
	return &ServiceReconciler{
		client: client,
		scheme: scheme,
	}
}

func (r ServiceReconciler) Reconcile(ctx context.Context, redis *cachev1alpha1.Redis) error {
	//master svc
	masterSvcName := fmt.Sprintf("%s-master", redis.Name)
	err := r.createOrUpdateService(ctx, masterSvcName, redis)
	if err != nil {
		return err
	}
	//replicas svc
	replicaSvcName := fmt.Sprintf("%s-replica", redis.Name)
	err = r.createOrUpdateService(ctx, replicaSvcName, redis)
	if err != nil {
		return err
	}
	headlessSvcName := fmt.Sprintf("%s-headless", redis.Name)
	err = r.createOrUpdateService(ctx, headlessSvcName, redis)
	if err != nil {
		return err
	}
	return nil
}

func (r ServiceReconciler) createOrUpdateService(ctx context.Context, svcName string, redis *cachev1alpha1.Redis) error {
	labels := util.GetLabels(redis, util.MasterLabels)
	svc := generateService(labels, svcName, redis.Namespace)

	_, err := controllerutil.CreateOrUpdate(ctx, *r.client, svc, func() error {
		// we generate the service again to override any change that might've been done to the live resource
		svc = generateService(labels, svcName, redis.Namespace)
		svc.ObjectMeta.SetLabels(labels)
		return controllerutil.SetControllerReference(redis, svc, r.scheme)
	})
	return err
}

func generateService(labels map[string]string, name string, namespace string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type: "ClusterIP",
			Ports: []corev1.ServicePort{
				{
					Name:       "tcp-redis",
					Port:       RedisPort,
					TargetPort: intstr.FromString(RedisPortName),
				},
			},
			Selector: labels,
		},
	}
}
