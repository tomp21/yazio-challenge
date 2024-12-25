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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cachev1alpha1 "github.com/tomp21/yazio-challenge/api/v1alpha1"
)

var _ = Describe("Redis Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		redis := &cachev1alpha1.Redis{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Redis")
			err := k8sClient.Get(ctx, typeNamespacedName, redis)
			if err != nil && errors.IsNotFound(err) {
				resource := &cachev1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: cachev1alpha1.RedisSpec{
						Replicas:      2,
						Version:       "7.4.1",
						VolumeStorage: resource.MustParse("2Gi"),
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &cachev1alpha1.Redis{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Redis")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &RedisReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			redis := &cachev1alpha1.Redis{}
			getError := k8sClient.Get(ctx, typeNamespacedName, redis)
			conditionAvailable := metav1.Condition{Type: conditionAvailable, Status: metav1.ConditionTrue, Reason: "Reconciled", Message: "Reconciliation finished"}

			Expect(err).NotTo(HaveOccurred())
			Expect(getError).NotTo(HaveOccurred())
			Expect(redis.Status.Conditions[0].Type).To(Equal(conditionAvailable.Type))
			Expect(redis.Status.Conditions[0].Status).To(Equal(conditionAvailable.Status))

		})
	})
})
