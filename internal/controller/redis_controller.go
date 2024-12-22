/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cachev1alpha1 "github.com/tomp21/yazio-challenge/api/v1alpha1"
)

const (
	conditionAvailable string = "Available"
	redisFinalizer            = "redis.yazio.com"
	redisPortName             = "redis"
	redisPort                 = 6379
)

var CommonLabels = map[string]string{
	"app.kubernetes.io/managed-by": "redis-operator",
	"app.kubernetes.io/name":       "redis",
}

var MasterLabels = map[string]string{
	"app.kubernetes.io/component": "master",
}

var ReplicaLabels = map[string]string{
	"app.kubernetes.io/component": "replica",
}

// RedisReconciler reconciles a Redis object
type RedisReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=cache.yazio.com,resources=redis,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cache.yazio.com,resources=redis/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cache.yazio.com,resources=redis/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// Permissions to manage ServiceAccounts
//+kubebuilder:rbac:groups="core",resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// Permissions to manage Services
//+kubebuilder:rbac:groups="core",resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Redis object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/reconcile
func (r *RedisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	redis := &cachev1alpha1.Redis{}

	err := r.Get(ctx, req.NamespacedName, redis)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// in this case the CR was deleted between the reconcile request being sent and our check, we can ignore this request
			log.Info(fmt.Sprintf("Redis resource not found: %v", req.NamespacedName))
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get Redis instance")
		return ctrl.Result{}, err
	}

	// If the Status of the CR is not set, this means this reconcile is the first one, therefore we update the status subresource to be unknown, and we retrieve the full CR again to ensure we have the latest object
	if redis.Status.Conditions == nil || len(redis.Status.Conditions) == 0 {
		meta.SetStatusCondition(&redis.Status.Conditions, metav1.Condition{Type: conditionAvailable, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})
		if err = r.Update(ctx, redis); err != nil {
			log.Error(err, "Failed to update redis resource")
			return ctrl.Result{}, err
		}
		if err = r.Get(ctx, req.NamespacedName, redis); err != nil {
			log.Error(err, "Failed to retrieve redis resource")
			return ctrl.Result{}, err
		}
	}

	// AddFinalizer will return true if the redis object didn't have this finalizer already, in which case we should add it
	if controllerutil.AddFinalizer(redis, redisFinalizer) {
		if err = r.Update(ctx, redis); err != nil {
			log.Error(err, "Could not update finalizers")
			return ctrl.Result{}, err
		}
	}

	// Check if the resource has a deletion timestamp, which is the way of marking an object to be deleted
	if redis.ObjectMeta.GetDeletionTimestamp() != nil {
		return r.handleFinalizer(ctx, redis)
	}

	// After making sure we have something to work on and it is not already marked for deletion, we can proceed to create or update it
	return r.CreateOrUpdateRedis(ctx, redis)
}

// SetupWithManager sets up the controller with the Manager.
func (r *RedisReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cachev1alpha1.Redis{}).
		Complete(r)
}

// Non-boilerplate functions

// Handles removal of finalizer, to be triggered on deletion, and if extra events need to be triggered on deletion, this is where to do so
func (r *RedisReconciler) handleFinalizer(ctx context.Context, redis *cachev1alpha1.Redis) (ctrl.Result, error) {
	log.Log.Info(fmt.Sprintf("Deleting resource %v/%v", redis.Namespace, redis.Name))
	controllerutil.RemoveFinalizer(redis, redisFinalizer)
	err := r.Update(ctx, redis)
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *RedisReconciler) CreateOrUpdateRedis(ctx context.Context, redis *cachev1alpha1.Redis) (ctrl.Result, error) {
	// For the sake of minimalism on this exercise, i'm leaving out (initially at least)
	// - NetworkPolicy
	// - PDBs
	//
	// This will create:
	// PVCs
	// Secret
	// Redis statefulset
	// ..?

	initLabels(redis)

	//SA
	err := r.createOrUpdateServiceAccount(ctx, redis)
	if err != nil {
		return ctrl.Result{}, err
	}

	//Master service
	if err = r.createOrUpdateMasterService(ctx, redis); err != nil {
		return ctrl.Result{}, err
	}
	//Replicas service
	if err = r.createOrUpdateReplicasService(ctx, redis); err != nil {
		return ctrl.Result{}, err
	}
	//Headless service
	if err = r.createOrUpdateHeadlessService(ctx, redis); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *RedisReconciler) createOrUpdateServiceAccount(ctx context.Context, redis *cachev1alpha1.Redis) error {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redis.Name,
			Namespace: redis.Namespace,
			Labels:    CommonLabels,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, sa, func() error {
		return controllerutil.SetControllerReference(redis, sa, r.Scheme)
	})
	return err
}

func (r *RedisReconciler) createOrUpdateMasterService(ctx context.Context, redis *cachev1alpha1.Redis) error {
	labels := extraLabels(MasterLabels)
	svcName := fmt.Sprintf("%s-master", redis.Name)
	svc := generateRedisService(labels, svcName, redis.Namespace)

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		return controllerutil.SetControllerReference(redis, svc, r.Scheme)
	})
	return err
}

func (r *RedisReconciler) createOrUpdateReplicasService(ctx context.Context, redis *cachev1alpha1.Redis) error {
	labels := extraLabels(ReplicaLabels)
	svcName := fmt.Sprintf("%s-replicas", redis.Name)
	svc := generateRedisService(labels, svcName, redis.Namespace)

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		return controllerutil.SetControllerReference(redis, svc, r.Scheme)
	})
	return err
}

func (r *RedisReconciler) createOrUpdateHeadlessService(ctx context.Context, redis *cachev1alpha1.Redis) error {
	svcName := fmt.Sprintf("%s-headless", redis.Name)
	svc := generateRedisService(CommonLabels, svcName, redis.Namespace)
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		return controllerutil.SetControllerReference(redis, svc, r.Scheme)
	})
	return err
}

func extraLabels(extras map[string]string) map[string]string {
	for k, v := range CommonLabels {
		extras[k] = v
	}
	return extras
}

// This method is intended to add any dynamic values to the `CommonLabels` map, for now only the instance identifier
func initLabels(redis *cachev1alpha1.Redis) {
	// this label helps us differentiate and avoid collisions on label selectors with other hypothetical redises in the same ns
	CommonLabels["app.kubernetes.io/instance"] = redis.Name
}

func generateRedisService(labels map[string]string, name string, namespace string) *corev1.Service {
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
					Port:       redisPort,
					TargetPort: intstr.FromString(redisPortName),
				},
			},
			Selector: labels,
		},
	}
}
