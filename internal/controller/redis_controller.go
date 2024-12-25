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
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"math/rand/v2"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cachev1alpha1 "github.com/tomp21/yazio-challenge/api/v1alpha1"
	"github.com/tomp21/yazio-challenge/internal/controller/reconcilers"
	"github.com/tomp21/yazio-challenge/internal/util"
)

const (
	conditionAvailable     = "Available"
	redisFinalizer         = "redis.yazio.com"
	redisPortName          = "redis"
	redisPort              = 6379
	passwordSpecialChars   = "!@#$%^&*()_+-=[]{};':,./?~"
	passwordLetters        = "abcdefghijklmnopqrstuvwxyz"
	passwordNumbers        = "0123456789"
	passwordLength         = 12
	redisImage             = "bitnami/redis"
	redisPasswordSecretKey = "redis-password"
	defaultMemoryRequest   = "512Mi"
	defaultCpuRequest      = "100m"
	defaultMemoryLimit     = "512Mi"
)

var masterLabels = map[string]string{
	"app.kubernetes.io/component": "master",
}

var replicaLabels = map[string]string{
	"app.kubernetes.io/component": "replica",
}

//go:embed configmaps/scripts/start-master.sh
var masterStartScript string

//go:embed configmaps/scripts/start-replica.sh
var replicaStartScript string

//go:embed configmaps/conf/redis.conf
var redisAllConf string

//go:embed configmaps/conf/master.conf
var redisMasterConf string

//go:embed configmaps/conf/master.conf
var redisReplicaConf string

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
	fmt.Println("Reconcile triggered")
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
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
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

	//SA
	saReconciler := reconcilers.NewServiceAccountReconciler(&r.Client, r.Scheme)
	err := saReconciler.Reconcile(ctx, redis)
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

	//Secret
	if err = r.manageSecret(ctx, redis); err != nil {
		return ctrl.Result{}, err
	}

	//ConfigMaps
	if err = r.createOrUpdateConfigMaps(ctx, redis); err != nil {
		return ctrl.Result{}, err
	}

	//Master statefulset
	if err = r.createOrUpdateMasterSS(ctx, redis); err != nil {
		return ctrl.Result{}, err
	}
	//Replicas statefulset
	if err = r.createOrUpdateReplicasSS(ctx, redis); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *RedisReconciler) createOrUpdateMasterService(ctx context.Context, redis *cachev1alpha1.Redis) error {
	labels := util.GetLabels(redis, masterLabels)
	svcName := fmt.Sprintf("%s-master", redis.Name)
	svc := generateRedisService(labels, svcName, redis.Namespace)

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		return controllerutil.SetControllerReference(redis, svc, r.Scheme)
	})
	return err
}

func (r *RedisReconciler) createOrUpdateReplicasService(ctx context.Context, redis *cachev1alpha1.Redis) error {
	labels := util.GetLabels(redis, replicaLabels)
	svcName := fmt.Sprintf("%s-replicas", redis.Name)
	svc := generateRedisService(labels, svcName, redis.Namespace)

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		return controllerutil.SetControllerReference(redis, svc, r.Scheme)
	})
	return err
}

func (r *RedisReconciler) createOrUpdateHeadlessService(ctx context.Context, redis *cachev1alpha1.Redis) error {
	svcName := fmt.Sprintf("%s-headless", redis.Name)
	svc := generateRedisService(util.GetLabels(redis, nil), svcName, redis.Namespace)
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		return controllerutil.SetControllerReference(redis, svc, r.Scheme)
	})
	return err
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

// Known issue: if the secret was edited and the redis-secret field no longer exist, but the secret itself is there, we are not fixing it
func (r *RedisReconciler) manageSecret(ctx context.Context, redis *cachev1alpha1.Redis) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redis.Name,
			Namespace: redis.Namespace,
			Labels:    util.GetLabels(redis, nil),
		},
	}
	namespacedName := types.NamespacedName{
		Name:      redis.Name,
		Namespace: redis.Namespace,
	}
	password := generateSecureRedisPassword()
	err := r.Get(ctx, namespacedName, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			secret.StringData = map[string]string{redisPasswordSecretKey: password}
			controllerutil.SetControllerReference(redis, secret, r.Scheme)
			return r.Create(ctx, secret)
		}
		return err
	}
	return nil
}

func (r *RedisReconciler) createOrUpdateMasterSS(ctx context.Context, redis *cachev1alpha1.Redis) error {
	// hardcoded to 1 since we currently only aim to support single master deployments
	masterSize := int32(1)
	defaultCMMode := int32(0755)
	volumeStorage, err := resource.ParseQuantity("2Gi")
	if err != nil {
		return err
	}
	labels := util.GetLabels(redis, masterLabels)
	imageFullName := fmt.Sprintf("%s:%s", redisImage, redis.Spec.Version)
	// Missing
	// - Security context
	// - Requests
	// - Probes
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-master", redis.Name),
			Namespace: redis.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &masterSize,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: redis.Name,
					Containers: []corev1.Container{
						{
							Name:    "redis",
							Image:   imageFullName,
							Command: []string{"/bin/bash"},
							Args:    []string{"-c", "/opt/bitnami/scripts/start-scripts/start-master.sh"},
							Env: []corev1.EnvVar{
								{
									Name: "REDIS_PASSWORD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: redis.Name,
											},
											Key: redisPasswordSecretKey,
										},
									},
								},
								{
									Name:  "REDIS_REPLICATION_MODE",
									Value: "master",
								},
								{
									Name:  "REDIS_PORT",
									Value: strconv.Itoa(redisPort),
								},
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "redis",
									ContainerPort: redisPort,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "start-scripts",
									MountPath: "/opt/bitnami/scripts/start-scripts",
								}, {
									Name:      "redis-conf",
									MountPath: "/opt/bitnami/redis/mounted-etc",
								},
								{
									Name:      fmt.Sprintf("%s-data", redis.Name),
									MountPath: "/data",
								},
							},
							Resources: getResources(),
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "start-scripts",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "start-scripts",
									},
									DefaultMode: &defaultCMMode,
								},
							},
						}, {
							Name: "redis-conf",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "redis-conf",
									},
									DefaultMode: &defaultCMMode,
								},
							},
						},
					},
				},
			},
		},
	}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, statefulSet, func() error {
		statefulSet.Spec.Template.Spec.Containers[0].Image = imageFullName
		statefulSet.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{
			{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "PersistentVolumeClaim",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:   fmt.Sprintf("%s-data", redis.Name),
					Labels: util.GetLabels(redis, nil),
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: volumeStorage,
						},
					},
				},
			},
		}
		return controllerutil.SetControllerReference(redis, statefulSet, r.Scheme)
	})

	return err
}

func (r *RedisReconciler) createOrUpdateReplicasSS(ctx context.Context, redis *cachev1alpha1.Redis) error {
	replicas := redis.Spec.Replicas
	fmt.Println("replicas")
	fmt.Println(replicas)
	defaultCMMode := int32(0755)
	volumeStorage, err := resource.ParseQuantity("2Gi")
	if err != nil {
		return err
	}
	labels := util.GetLabels(redis, masterLabels)
	imageFullName := fmt.Sprintf("%s:%s", redisImage, redis.Spec.Version)
	// Missing
	// - Security context
	// - Requests
	// - Probes
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-replica", redis.Name),
			Namespace: redis.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: redis.Name,
					Containers: []corev1.Container{
						{
							Name:    "redis",
							Image:   imageFullName,
							Command: []string{"/bin/bash"},
							Args:    []string{"-c", "/opt/bitnami/scripts/start-scripts/start-replica.sh"},
							Env: []corev1.EnvVar{
								{
									Name: "REDIS_PASSWORD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: redis.Name,
											},
											Key: redisPasswordSecretKey,
										},
									},
								},
								{
									Name:  "REDIS_REPLICATION_MODE",
									Value: "replica",
								},
								{
									Name:  "REDIS_MASTER_HOST",
									Value: fmt.Sprintf("%s-master", redis.Name),
								},
								{
									Name:  "REDIS_PORT",
									Value: strconv.Itoa(redisPort),
								},
								{
									Name:  "HEADLESS_SERVICE",
									Value: fmt.Sprintf("%s-headless", redis.Name),
								},
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "redis",
									ContainerPort: redisPort,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "start-scripts",
									MountPath: "/opt/bitnami/scripts/start-scripts",
								}, {
									Name:      "redis-conf",
									MountPath: "/opt/bitnami/redis/mounted-etc",
								},
								{
									Name:      fmt.Sprintf("%s-data", redis.Name),
									MountPath: "/data",
								},
							},
							Resources: getResources(),
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "start-scripts",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "start-scripts",
									},
									DefaultMode: &defaultCMMode,
								},
							},
						}, {
							Name: "redis-conf",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "redis-conf",
									},
									DefaultMode: &defaultCMMode,
								},
							},
						},
					},
				},
			},
		},
	}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, statefulSet, func() error {
		statefulSet.Spec.Replicas = &replicas
		statefulSet.Spec.Template.Spec.Containers[0].Image = imageFullName
		statefulSet.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{
			{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "PersistentVolumeClaim",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:   fmt.Sprintf("%s-data", redis.Name),
					Labels: util.GetLabels(redis, nil),
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: volumeStorage,
						},
					},
				},
			},
		}
		return controllerutil.SetControllerReference(redis, statefulSet, r.Scheme)
	})
	return err
}

func (r *RedisReconciler) createOrUpdateConfigMaps(ctx context.Context, redis *cachev1alpha1.Redis) error {

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
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, cmStartScripts, func() error {
		return controllerutil.SetControllerReference(redis, cmStartScripts, r.Scheme)
	})
	if err != nil {
		return err
	}

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

	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, cmConfig, func() error {
		return controllerutil.SetControllerReference(redis, cmConfig, r.Scheme)
	})

	if err != nil {
		return err
	}

	return nil
}

// Making this a method in case this needs to be calculated in some other way in the future
func getResources() corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse(defaultMemoryRequest),
			corev1.ResourceCPU:    resource.MustParse(defaultCpuRequest),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse(defaultMemoryLimit),
		},
	}
}

func generateSecureRedisPassword() string {
	var password []byte
	//TODO improve this, it works fine but the code is ugly and will only have 1 uppercase character, though it is good enough for now
	// Though it ensures it has at least 1 upppercase char, 1 lowercase char and a special char
	password = append(password, passwordSpecialChars[rand.IntN(len(passwordSpecialChars)-1)])
	password = append(password, passwordLetters[rand.IntN(len(passwordLetters)-1)])
	password = bytes.ToUpper(password)
	password = append(password, passwordLetters[rand.IntN(len(passwordLetters)-1)])
	password = append(password, passwordNumbers[rand.IntN(len(passwordNumbers)-1)])
	allchars := passwordSpecialChars + passwordLetters + passwordNumbers
	for len(password) < passwordLength {
		password = append(password, allchars[rand.IntN(len(allchars)-1)])
	}
	rand.Shuffle(len(password), func(i, j int) {
		password[i], password[j] = password[j], password[i]
	})
	return string(password)
}
