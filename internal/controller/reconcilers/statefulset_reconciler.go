package reconcilers

import (
	"context"
	"fmt"
	"strconv"

	cachev1alpha1 "github.com/tomp21/yazio-challenge/api/v1alpha1"
	"github.com/tomp21/yazio-challenge/internal/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type StatefulSetReconciler struct {
	client.Client
	scheme *runtime.Scheme
}

const (
	defaultMemoryRequest = "512Mi"
	defaultCpuRequest    = "100m"
	defaultMemoryLimit   = "512Mi"
	redisImage           = "bitnami/redis"
)

func NewStatefulSetReconciler(client client.Client, scheme *runtime.Scheme) *StatefulSetReconciler {
	return &StatefulSetReconciler{
		Client: client,
		scheme: scheme,
	}
}

func (r *StatefulSetReconciler) Reconcile(ctx context.Context, redis *cachev1alpha1.Redis) error {
	err := r.handleMasterStatefulSet(ctx, redis)
	if err != nil {
		return err
	}
	err = r.handleReplicaStatefulSet(ctx, redis)
	if err != nil {
		return err
	}
	return nil
}

func (r *StatefulSetReconciler) handleReplicaStatefulSet(ctx context.Context, redis *cachev1alpha1.Redis) error {
	statefulSet := &appsv1.StatefulSet{}
	namespacedName := types.NamespacedName{Name: fmt.Sprintf("%s-replica", redis.Name), Namespace: redis.Namespace}
	replicaStatefulSet := generateReplicaStatefulSet(*statefulSet, redis)
	controllerutil.SetOwnerReference(redis, &replicaStatefulSet, r.scheme)

	err := r.Get(ctx, namespacedName, statefulSet)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		if err = r.Create(ctx, &replicaStatefulSet); err != nil {
			return err
		}

	} else {
		err = r.Update(ctx, &replicaStatefulSet)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *StatefulSetReconciler) handleMasterStatefulSet(ctx context.Context, redis *cachev1alpha1.Redis) error {
	statefulSet := &appsv1.StatefulSet{}
	namespacedName := types.NamespacedName{Name: fmt.Sprintf("%s-master", redis.Name), Namespace: redis.Namespace}
	masterStatefulSet := generateMasterStatefulSet(*statefulSet, redis)
	controllerutil.SetOwnerReference(redis, &masterStatefulSet, r.scheme)

	err := r.Get(ctx, namespacedName, statefulSet)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}

		if err = r.Create(ctx, &masterStatefulSet); err != nil {
			return err
		}

	} else {
		err = r.Update(ctx, &masterStatefulSet)
		if err != nil {
			return err
		}
	}

	return nil
}

func generateBaseStatefulSet(redis *cachev1alpha1.Redis) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redis.Name,
			Namespace: redis.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: redis.Name,
					Containers: []corev1.Container{
						{
							Name:  "redis",
							Image: "to-be-overwritten",
						},
					},
				},
			},
		},
	}
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

func generateReplicaStatefulSet(ss appsv1.StatefulSet, redis *cachev1alpha1.Redis) appsv1.StatefulSet {
	imageFullName := fmt.Sprintf("%s:%s", redisImage, redis.Spec.Version)
	labels := util.GetLabels(redis, util.ReplicaLabels)
	ss.ObjectMeta.SetName(fmt.Sprintf("%s-replica", redis.Name))
	ss.ObjectMeta.SetNamespace(redis.Namespace)
	ss.ObjectMeta.SetLabels(labels)
	ss.Spec.Replicas = &redis.Spec.Replicas
	ss.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: labels,
	}
	ss.Spec.Template.ObjectMeta.SetLabels(labels)
	ss.Spec.Template.Spec.Containers = getReplicaContainers(redis.Name, imageFullName)
	ss.Spec.Template.Spec.Volumes = getVolumes(redis.Name)
	ss.Spec.VolumeClaimTemplates = getVolumeClaimTemplates(redis, util.GetLabels(redis, nil))
	return ss
}

func generateMasterStatefulSet(ss appsv1.StatefulSet, redis *cachev1alpha1.Redis) appsv1.StatefulSet {
	imageFullName := fmt.Sprintf("%s:%s", redisImage, redis.Spec.Version)
	labels := util.GetLabels(redis, util.MasterLabels)
	masterReplicas := int32(1)
	ss.ObjectMeta.SetName(fmt.Sprintf("%s-master", redis.Name))
	ss.ObjectMeta.SetNamespace(redis.Namespace)
	ss.ObjectMeta.SetLabels(labels)
	ss.Spec.Replicas = &masterReplicas
	ss.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: labels,
	}
	ss.Spec.Template.ObjectMeta.SetLabels(labels)
	ss.Spec.Template.Spec.Containers = getMasterContainers(redis.Name, imageFullName)
	ss.Spec.Template.Spec.Volumes = getVolumes(redis.Name)
	ss.Spec.VolumeClaimTemplates = getVolumeClaimTemplates(redis, util.GetLabels(redis, nil))
	return ss
}

func getMasterContainers(name, image string) []corev1.Container {
	return []corev1.Container{
		{
			Name:    "redis",
			Image:   image,
			Command: []string{"/bin/bash"},
			Args:    []string{"-c", "/opt/bitnami/scripts/start-scripts/start-master.sh"},
			Env: []corev1.EnvVar{
				{
					Name: "REDIS_PASSWORD",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: name,
							},
							Key: PasswordSecretKey,
						},
					},
				},
				{
					Name:  "REDIS_REPLICATION_MODE",
					Value: "master",
				},
				{
					Name:  "REDIS_PORT",
					Value: strconv.Itoa(RedisPort),
				},
			},
			Ports: []corev1.ContainerPort{
				{
					Name:          "redis",
					ContainerPort: RedisPort,
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
					Name:      fmt.Sprintf("%s-data", name),
					MountPath: "/data",
				},
				{
					Name:      "health",
					MountPath: "/health",
				},
			},
			LivenessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					Exec: &corev1.ExecAction{
						Command: []string{
							"sh",
							"-c",
							"/health/ping_liveness_local.sh 5",
						},
					},
				},
			},
			ReadinessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					Exec: &corev1.ExecAction{
						Command: []string{
							"sh",
							"-c",
							"/health/ping_readiness_local.sh 1",
						},
					},
				},
			},
			Resources: getResources(),
		},
	}
}

func getReplicaContainers(name, image string) []corev1.Container {
	return []corev1.Container{
		{
			Name:    "redis",
			Image:   image,
			Command: []string{"/bin/bash"},
			Args:    []string{"-c", "/opt/bitnami/scripts/start-scripts/start-replica.sh"},
			Env: []corev1.EnvVar{
				{
					Name: "REDIS_PASSWORD",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: name,
							},
							Key: PasswordSecretKey,
						},
					},
				},
				{
					Name: "REDIS_MASTER_PASSWORD",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: name,
							},
							Key: PasswordSecretKey,
						},
					},
				},
				{
					Name:  "REDIS_REPLICATION_MODE",
					Value: "replica",
				},
				{
					Name:  "REDIS_MASTER_HOST",
					Value: fmt.Sprintf("%s-master", name),
				},
				{
					Name:  "REDIS_MASTER_PORT_NUMBER",
					Value: strconv.Itoa(RedisPort),
				},
				{
					Name:  "REDIS_PORT",
					Value: strconv.Itoa(RedisPort),
				},
				{
					Name:  "HEADLESS_SERVICE",
					Value: fmt.Sprintf("%s-headless", name),
				},
				{
					Name:  "ALLOW_EMPLTY_PASSWORD",
					Value: "no",
				},
			},
			Ports: []corev1.ContainerPort{
				{
					Name:          "redis",
					ContainerPort: RedisPort,
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
					Name:      fmt.Sprintf("%s-data", name),
					MountPath: "/data",
				},
				{
					Name:      "health",
					MountPath: "/health",
				},
			},
			LivenessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					Exec: &corev1.ExecAction{
						Command: []string{
							"sh",
							"-c",
							"/health/ping_liveness_local_and_master.sh 5",
						},
					},
				},
			},
			ReadinessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					Exec: &corev1.ExecAction{
						Command: []string{
							"sh",
							"-c",
							"/health/ping_readiness_local_and_master.sh 1",
						},
					},
				},
			},
			Resources: getResources(),
		},
	}
}

func getVolumeClaimTemplates(redis *cachev1alpha1.Redis, labels map[string]string) []corev1.PersistentVolumeClaim {
	return []corev1.PersistentVolumeClaim{
		{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "PersistentVolumeClaim",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:   fmt.Sprintf("%s-data", redis.Name),
				Labels: labels,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: redis.Spec.VolumeStorage,
					},
				},
			},
		},
	}
}

func getVolumes(instanceName string) []corev1.Volume {
	defaultMode := int32(0755)
	return []corev1.Volume{
		{
			Name: "start-scripts",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: fmt.Sprintf("%s-start-scripts", instanceName),
					},
					DefaultMode: &defaultMode,
				},
			},
		}, {
			Name: "redis-conf",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: fmt.Sprintf("%s-conf", instanceName),
					},
					DefaultMode: &defaultMode,
				},
			},
		}, {
			Name: "health",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: fmt.Sprintf("%s-health", instanceName),
					},
					DefaultMode: &defaultMode,
				},
			},
		},
	}
}
