package util

import (
	cachev1alpha1 "github.com/tomp21/yazio-challenge/api/v1alpha1"
)

var baseLabels = map[string]string{
	"app.kubernetes.io/managed-by": "redis-operator",
	"app.kubernetes.io/name":       "redis",
}

var MasterLabels = map[string]string{
	"app.kubernetes.io/component": "master",
}

var ReplicaLabels = map[string]string{
	"app.kubernetes.io/component": "replica",
}

func GetLabels(redis *cachev1alpha1.Redis, extra map[string]string) map[string]string {
	labels := make(map[string]string)
	for k, v := range baseLabels {
		labels[k] = v
	}
	// this label helps us differentiate and avoid collisions on label selectors with other hypothetical redises in the same ns
	labels["app.kubernetes.io/instance"] = redis.Name
	if extra != nil {
		for k, v := range extra {
			labels[k] = v
		}
	}
	return labels
}
