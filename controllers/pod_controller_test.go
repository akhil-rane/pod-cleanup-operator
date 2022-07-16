/*
Copyright 2022.

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

package controllers

import (
	"context"
	"github.com/akhil-rane/pod-cleanup-operator/api/v1alpha1"
	log "github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	//+kubebuilder:scaffold:imports
)

const (
	// TestPodName is a name of a test pod in bad or completed state
	TestPodName = "test-pod-name"
	// TestPodNamespace is a namespace of a test pod in bad or completed state
	TestPodNamespace = "test-pod-namespace"
)

func TestReconcile(t *testing.T) {
	log.Info("setting up scheme")
	if err := v1alpha1.AddToScheme(scheme.Scheme); err != nil {
		log.WithError(err).Fatal("unable to add APIs to scheme")
	}

	tests := []struct {
		name                 string
		existing             []runtime.Object
		expectErr            bool
		validate             func(client.Client, *testing.T)
		expectedDeletion     bool
		validateRequeueAfter func(time.Duration, client.Client, *testing.T)
	}{
		{
			name: "successful pod with deletion delay satisfied, deletion mode normal",
			existing: []runtime.Object{
				testOperatorConfig(v1alpha1.DeletionModeNormal, 2),
				testPod(corev1.PodSucceeded, time.Now().Add(-3*time.Minute)),
			},
			expectErr:        false,
			expectedDeletion: true,
		},
		{
			name: "failed pod with deletion delay satisfied, deletion mode normal",
			existing: []runtime.Object{
				testOperatorConfig(v1alpha1.DeletionModeNormal, 2),
				testPod(corev1.PodFailed, time.Now().Add(-3*time.Minute)),
			},
			expectErr:        false,
			expectedDeletion: true,
		},
		{
			name: "running pod with deletion delay satisfied, deletion mode normal",
			existing: []runtime.Object{
				testOperatorConfig(v1alpha1.DeletionModeNormal, 2),
				testPod(corev1.PodRunning, time.Now().Add(-3*time.Minute)),
			},
			expectErr:        false,
			expectedDeletion: false,
		},
		{
			name: "pod cleanup operator config does not exists",
			existing: []runtime.Object{
				testPod(corev1.PodSucceeded, time.Now().Add(-3*time.Minute)),
			},
			expectErr:        true,
			expectedDeletion: false,
		},
		{
			name: "pod does not exists, got deleted midway through reconciliation",
			existing: []runtime.Object{
				testOperatorConfig(v1alpha1.DeletionModeNormal, 2),
			},
			expectErr:        false,
			expectedDeletion: true,
		},
		{
			name: "successful pod with deletion delay satisfied, deletion mode force",
			existing: []runtime.Object{
				testOperatorConfig(v1alpha1.DeletionModeForce, 2),
				testPod(corev1.PodSucceeded, time.Now().Add(-3*time.Minute)),
			},
			expectErr:        false,
			expectedDeletion: true,
		},
		{
			name: "successful pod with deletion delay satisfied, deletion mode skip",
			existing: []runtime.Object{
				testOperatorConfig(v1alpha1.DeletionModeSkip, 2),
				testPod(corev1.PodSucceeded, time.Now().Add(-3*time.Minute)),
			},
			expectErr:        false,
			expectedDeletion: false,
		},
		{
			name: "successful pod with deletion delay not satisfied, deletion mode normal",
			existing: []runtime.Object{
				testOperatorConfig(v1alpha1.DeletionModeNormal, 5),
				testPod(corev1.PodSucceeded, time.Now().Add(-3*time.Minute)),
			},
			expectErr:        false,
			expectedDeletion: false,
			validateRequeueAfter: func(requeueAfter time.Duration, c client.Client, t *testing.T) {
				pod := &corev1.Pod{}
				err := c.Get(context.TODO(), client.ObjectKey{Name: TestPodName, Namespace: TestPodNamespace}, pod)
				assert.NoError(t, err, "Unexpected error fetching pod: %v", err)

				operatorConfig := &v1alpha1.PodCleanupConfig{}
				err = c.Get(context.TODO(), client.ObjectKey{Name: PodCleanupOperatorConfig, Namespace: PodCleanupOperatorNamespace}, operatorConfig)
				assert.NoError(t, err, "Unexpected error fetching pod cleanup config: %v", err)
				assert.Less(t, requeueAfter.Nanoseconds(), int64(operatorConfig.Spec.DeletionDelayInMinutes)*time.Minute.Nanoseconds(), "unexpected requeue after duration")
				assert.Greater(t, requeueAfter.Nanoseconds(), pod.CreationTimestamp.Add(time.Duration(operatorConfig.Spec.DeletionDelayInMinutes)*time.Minute).Sub(time.Now()).Nanoseconds(),
					"unexpected requeue after duration")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			fakeClient := fake.NewClientBuilder().WithRuntimeObjects(test.existing...).Build()
			pr := &PodReconciler{
				Client: fakeClient,
			}

			result, err := pr.Reconcile(context.TODO(), reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      TestPodName,
					Namespace: TestPodNamespace,
				},
			})

			if err != nil && !test.expectErr {
				assert.NoError(t, err, "Unexpected error: %v", err)
			}
			if err == nil && test.expectErr {
				assert.Error(t, err, "Expected error but got none")
			}

			pod := &corev1.Pod{}
			err = fakeClient.Get(context.TODO(), client.ObjectKey{Name: TestPodName, Namespace: TestPodNamespace}, pod)

			if test.expectedDeletion {
				assert.Error(t, err, "Expected error but got none")
				assert.True(t, apierrors.IsNotFound(err), "Expected not found error but got a different one %v", err)
			} else {
				assert.Equal(t, TestPodName, pod.Name, "Got unexpected pod name %v", pod.Name)
				assert.Equal(t, TestPodNamespace, pod.Namespace, "Got unexpected pod name %v", pod.Namespace)
			}
			if test.validateRequeueAfter != nil {
				test.validateRequeueAfter(result.RequeueAfter, fakeClient, t)
			}

		})
	}
}

func testPod(podPhase corev1.PodPhase, creationTime time.Time) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              TestPodName,
			Namespace:         TestPodNamespace,
			CreationTimestamp: metav1.NewTime(creationTime),
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "main",
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: podPhase,
		},
	}

	return pod
}

func testOperatorConfig(deletionMode v1alpha1.DeletionMode, deletionDelayInMinutes int32) *v1alpha1.PodCleanupConfig {
	operatorConfig := &v1alpha1.PodCleanupConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PodCleanupOperatorConfig,
			Namespace: PodCleanupOperatorNamespace,
		},
		Spec: v1alpha1.PodCleanupConfigSpec{
			DeletionMode:           deletionMode,
			DeletionDelayInMinutes: deletionDelayInMinutes,
		},
	}
	return operatorConfig
}
