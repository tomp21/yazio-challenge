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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cachev1alpha1 "github.com/tomp21/yazio-challenge/api/v1alpha1"
	"github.com/tomp21/yazio-challenge/internal/controller/reconcilers"
)

const (
	conditionAvailable = "Available"
	redisFinalizer     = "redis.yazio.com"
	redisImage         = "bitnami/redis"
)

var masterLabels = map[string]string{
	"app.kubernetes.io/component": "master",
}

var replicaLabels = map[string]string{
	"app.kubernetes.io/component": "replica",
}

// RedisReconciler reconciles a Redis object
type RedisReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=cache.yazio.com,resources=redis,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cache.yazio.com,resources=redis/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cache.yazio.com,resources=redis/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// Permissions to manage ServiceAccounts
//+kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// Permissions to manage Services
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//Permissions to manage Secrets
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//Permissions to manage StatefulSets
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//Permissions to manage ConfigMaps
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
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
		if err = r.Status().Update(ctx, redis); err != nil {
			log.Error(err, "Failed to update redis resource")
			return ctrl.Result{}, err
		}
		if err = r.Get(ctx, req.NamespacedName, redis); err != nil {
			log.Error(err, "Failed to retrieve redis resource")
			return ctrl.Result{}, err
		}
	}

	// AddFinalizer will return true if the redis object didn't have this finalizer already, in which case we should add it
	if !controllerutil.ContainsFinalizer(redis, redisFinalizer) {
		if ok := controllerutil.AddFinalizer(redis, redisFinalizer); !ok {
			log.Error(err, "Failed to add finalizer into the custom resource")
			return ctrl.Result{Requeue: true}, nil
		}
		if err = r.Update(ctx, redis); err != nil {
			log.Error(err, "Failed to update custom resource to add finalizer")
			return ctrl.Result{}, err
		}
	}

	// Check if the resource has a deletion timestamp, which is the way of marking an object to be deleted
	if redis.ObjectMeta.GetDeletionTimestamp() != nil {
		if err = r.handleFinalizer(ctx, redis); err != nil {
			return ctrl.Result{}, err
		}
	}

	// After making sure we have something to work on and it is not already marked for deletion, we can proceed to create or update it
	err = r.CreateOrUpdateRedis(ctx, redis)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Once all operations are done, we set the Available condition to True
	if err = r.Get(ctx, req.NamespacedName, redis); err != nil {
		log.Error(err, "Failed to retrieve redis resource")
		return ctrl.Result{}, err
	}
	meta.SetStatusCondition(&redis.Status.Conditions, metav1.Condition{Type: conditionAvailable, Status: metav1.ConditionTrue, Reason: "Reconciled", Message: "Reconciliation finished"})
	if err = r.Status().Update(ctx, redis); err != nil {
		log.Error(err, "Failed to set status condition for redis resource")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RedisReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cachev1alpha1.Redis{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

// Non-boilerplate functions

// Handles removal of finalizer, to be triggered on deletion, and if extra events need to be triggered on deletion, this is where to do so
func (r *RedisReconciler) handleFinalizer(ctx context.Context, redis *cachev1alpha1.Redis) error {
	log.Log.Info(fmt.Sprintf("Deleting resource %v/%v", redis.Namespace, redis.Name))
	controllerutil.RemoveFinalizer(redis, redisFinalizer)
	err := r.Update(ctx, redis)
	if err != nil {
		return err
	}
	return nil
}

func (r *RedisReconciler) CreateOrUpdateRedis(ctx context.Context, redis *cachev1alpha1.Redis) error {
	// For the sake of minimalism on this exercise, i'm leaving out
	// - NetworkPolicy
	// - PDBs

	//SA
	saReconciler := reconcilers.NewServiceAccountReconciler(&r.Client, r.Scheme)
	err := saReconciler.Reconcile(ctx, redis)
	if err != nil {
		return err
	}

	//Secret
	secretReconciler := reconcilers.NewSecretReconciler(r.Client, r.Scheme)
	if err = secretReconciler.Reconcile(ctx, redis); err != nil {
		return err
	}

	svcReconciler := reconcilers.NewServiceReconciler(&r.Client, r.Scheme)
	if err = svcReconciler.Reconcile(ctx, redis); err != nil {
		return err
	}

	//ConfigMaps
	cmReconciler := reconcilers.NewConfigMapReconciler(&r.Client, r.Scheme)
	if err = cmReconciler.Reconcile(ctx, redis); err != nil {
		return err
	}

	statefulSetReconciler := reconcilers.NewStatefulSetReconciler(r.Client, r.Scheme)
	if err = statefulSetReconciler.Reconcile(ctx, redis); err != nil {
		return err
	}
	return nil
}
