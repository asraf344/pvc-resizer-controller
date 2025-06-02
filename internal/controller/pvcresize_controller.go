/*
Copyright 2025.

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
	"time"

	corev1 "k8s.io/api/core/v1"
	resource "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1alpha1 "github.com/makro/pvc-resizer-controller/api/v1alpha1"
)

// PVCResizeReconciler reconciles a PVCResize object
type PVCResizeReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:namespace=system,groups="",resources=pvcs,verbs=get;list;update;watch
//+kubebuilder:rbac:namespace=system,groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=volume.makro.com,resources=pvcresizes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=volume.makro.com,resources=pvcresizes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=volume.makro.com,resources=pvcresizes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the PVCResize object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *PVCResizeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var pvcResize v1alpha1.PVCResize
	err := r.Get(ctx, req.NamespacedName, &pvcResize)
	if err != nil {
		// pvcResize might be deleted
		logger.Info("pvcResize deleted or not found", "name", req.NamespacedName)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconciling pvcResize", "name", pvcResize.Name)

	err = r.resizePVC(ctx, req, pvcResize)
	if err != nil {
		return ctrl.Result{}, err
	}

	// For now Reconcile after 10s to check if the size changes at PVC, anyway
	return ctrl.Result{RequeueAfter: time.Second * 10}, nil
}

func (r *PVCResizeReconciler) resizePVC(ctx context.Context, req ctrl.Request, pvcResize v1alpha1.PVCResize) error {
	for _, policy := range pvcResize.Spec.Policies {
		var pvc corev1.PersistentVolumeClaim

		pvcKey := client.ObjectKey{Name: policy.Ref, Namespace: req.Namespace}
		err := r.Client.Get(ctx, pvcKey, &pvc)
		if err != nil {
			return err
		}

		newSize := resource.MustParse(policy.Size)
		// update the pvc only if size changes
		if !pvc.Spec.Resources.Requests[corev1.ResourceStorage].Equal(newSize) {
			pvc.Spec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse(policy.Size)
			// update pvc
			err = r.Client.Update(ctx, &pvc)
			if err != nil {
				return err
			}
		}

	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PVCResizeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.PVCResize{}).
		Complete(r)
}
