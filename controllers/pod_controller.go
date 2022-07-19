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
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/akhil-rane/pod-cleanup-operator/api/v1alpha1"
)

// PodReconciler reconciles a Pod object
type PodReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const (
	// PodCleanupOperatorConfig is the default name of the config for pod cleanup operator
	PodCleanupOperatorConfig = "pod-cleanup-operator-config"
	// PodCleanupOperatorNamespace is the default namespace for pod cleanup operator
	PodCleanupOperatorNamespace = "pod-cleanup-operator-system"
	// PodsListPageSize is the default pagination size when queuing pods in the cluster when
	// operator config changes
	PodsListPageSize = 1000
)

//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;delete
//+kubebuilder:rbac:groups=core,resources=pods/status,verbs=get
//+kubebuilder:rbac:groups=operator.craftdemo.com,resources=podcleanupconfigs,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Pod object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.1/pkg/reconcile
func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.WithFields(log.Fields{
		"pod": fmt.Sprintf("%s/%s", req.NamespacedName.Namespace, req.NamespacedName.Name),
	})

	deletionMode, deletionDelayInMinutes, err := getOperatorConfiguration(r.Client, logger)
	if err != nil {
		logger.WithError(err).Error("Error fetching operator configuration")
		return ctrl.Result{}, err
	}

	if deletionMode == v1alpha1.DeletionModeSkip {
		logger.Info("Skipping pod cleanup as deletion mode is set to SKIP")
		return ctrl.Result{}, err
	}

	var pod corev1.Pod
	if err := r.Get(ctx, req.NamespacedName, &pod); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Pod no longer exists. It is already deleted")
			return ctrl.Result{}, nil
		}
		logger.WithError(err).Error("Error fetching Pod")
		return ctrl.Result{}, err
	}

	if isBadOrCompletedPod(pod) {
		logger.Info("Pod is in bad or completed state")
		// if pod is not in bad or completed state for more than minutes specified by deletion delay, requeue after the
		// calculated delta time
		if time.Since(pod.CreationTimestamp.Time) < (time.Duration(deletionDelayInMinutes) * time.Minute) {
			logger.Info("Deletion delay not satisfied for this pod yet. Requeuing...")
			// podDeletionRecheckDelay will be duration between current time and expected pod deletion time (pod creation time + deletion delay)
			podDeletionRecheckDelay := pod.CreationTimestamp.Time.Add(time.Duration(deletionDelayInMinutes) * time.Minute).Sub(time.Now())
			return ctrl.Result{RequeueAfter: podDeletionRecheckDelay}, nil
		}

		var deleteOptions client.DeleteOptions
		if deletionMode == v1alpha1.DeletionModeForce {
			// Set deletion grace period to zero when deletion mode is set to FORCE
			logger.Info("Deletion mode is set to FORCE")
			var zeroGracePeriod int64 = 0
			deleteOptions = client.DeleteOptions{GracePeriodSeconds: &zeroGracePeriod}
		}
		logger.Info("Attempting pod deletion")
		if err := r.Delete(ctx, &pod, &deleteOptions); err != nil {
			if apierrors.IsNotFound(err) {
				logger.Info("Pod no longer exists. It is already deleted")
				return ctrl.Result{}, nil
			}
			logger.WithError(err).Error("Error deleting Pod")
			return ctrl.Result{}, nil
		}
		metricClustersDeleted.Inc()
		return ctrl.Result{}, nil
	}

	logger.Info("Pod is not in bad or completed state. Skipping deletion")
	return ctrl.Result{}, nil
}

// isBadOrCompletedPod checks if pod is in bad or completed state
// There are 3 scenarios of this state
// 1) Pod has failed
// 2) Pod has completed successfully
// 3) Pod is running but has deletionTimestamp set i.e. it is terminating
func isBadOrCompletedPod(pod corev1.Pod) bool {
	if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
		return true
	} else if pod.Status.Phase == corev1.PodRunning && pod.DeletionTimestamp != nil {
		return true
	}
	return false
}

// getOperatorConfiguration fetches pod cleanup operator config
func getOperatorConfiguration(kubeClient client.Client, logger log.FieldLogger) (v1alpha1.DeletionMode, int32, error) {
	logger.Info("Fetching deletion mode and deletion delay from operator config")
	config := &v1alpha1.PodCleanupConfig{}
	err := kubeClient.Get(context.TODO(),
		types.NamespacedName{
			Name:      PodCleanupOperatorConfig,
			Namespace: PodCleanupOperatorNamespace,
		}, config)
	if err != nil {
		return "", 0, err
	}

	return config.Spec.DeletionMode, config.Spec.DeletionDelayInMinutes, nil
}

// isOperatorConfig returns true if the name of given config is pod-cleanup-operator-config
func isOperatorConfig(cm metav1.Object) bool {
	return cm.GetName() == PodCleanupOperatorConfig && cm.GetNamespace() == PodCleanupOperatorNamespace
}

// SetupWithManager sets up the controller with the Manager.
func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {

	// allPodsMapFn simply looks up all the Pods and requests they be reconciled.
	allPodsMapFn := handler.EnqueueRequestsFromMapFunc(func(a client.Object) []reconcile.Request {
		log.Info("requeueing all Pods")
		var requests []reconcile.Request
		// Large Kubernetes cluster can have around 5000 worker nodes and each node in popular kubernetes platforms
		// (like Google Kubernetes Engine) can run around 100 pods. This leads to worst case scenario of cluster running
		// around 500000 pods. If we consider maximum resource size allowed by etcd to be around 2 Mb, it can lead to out
		// of memory error if we list all the pods together. Hence we are opting for pagination here.
		listOptions := &client.ListOptions{Limit: PodsListPageSize}
		continueToken := "dummyContinueToken"
		for continueToken != "" {
			pods := &corev1.PodList{}
			err := mgr.GetClient().List(context.TODO(), pods, listOptions)
			if err != nil {
				log.WithError(err).Error("error listing all pods for requeue")
				return requests
			}
			for _, pod := range pods.Items {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      pod.Name,
						Namespace: pod.Namespace,
					},
				})
			}
			listOptions.Continue = pods.Continue
			continueToken = pods.Continue
		}

		return requests
	})

	// configPredicate checks if the event is for the pod-cleanup-operator-config
	configPredicate := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return isOperatorConfig(e.ObjectNew)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return isOperatorConfig(e.Object)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return isOperatorConfig(e.Object)
		},
	}

	// cleanupPredicate allows pod reconcile only in case of update event i.e. when pod changes to bad or completed state
	cleanupPredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(
			&corev1.Pod{},
			builder.WithPredicates(cleanupPredicate)).
		Watches(
			&source.Kind{Type: &v1alpha1.PodCleanupConfig{}},
			allPodsMapFn,
			builder.WithPredicates(configPredicate)).
		Complete(r)
}
