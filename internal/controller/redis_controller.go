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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cachev1alpha1 "github.com/tomp21/yazio-challenge/api/v1alpha1"
)

const (
	conditionAvailable string = "Available"
	redisFinalizer            = "redis-controller"
)

// RedisReconciler reconciles a Redis object
type RedisReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=cache.yazio.com,resources=redis,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cache.yazio.com,resources=redis/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cache.yazio.com,resources=redis/finalizers,verbs=update

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

func (r *RedisReconciler) handleFinalizer(ctx context.Context, redis *cachev1alpha1.Redis) (ctrl.Result, error) {
	// TODO
	return ctrl.Result{}, nil
}

func (r *RedisReconciler) CreateOrUpdateRedis(ctx context.Context, redis *cachev1alpha1.Redis) (ctrl.Result, error) {
	// For the sake of minimalism on this exercise, i'm leaving out (initially at least)
	// - NetworkPolicy
	//
	// This will create:
	// PVCs
	// Redis statefulset
	// Secret
	// Service to access Redis
	// ServiceAccount
	// ..?

	return ctrl.Result{}, nil
}
