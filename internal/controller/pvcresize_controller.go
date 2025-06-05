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
	"errors"
	"fmt"
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	resource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	v1alpha1 "github.com/makro/pvc-resizer-controller/api/v1alpha1"
	utils "github.com/makro/pvc-resizer-controller/internal"
)

// PVCResizeReconciler reconciles a PVCResize object
type PVCResizeReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	State State
}

type State struct {
	// The name and namespace of the resource being reconciled.
	ResourceName types.NamespacedName

	Context   context.Context
	Logger    logr.Logger
	PVCResize v1alpha1.PVCResize

	PVCs map[string]corev1.PersistentVolumeClaim
	PVs  map[string]corev1.PersistentVolume

	PoliciesToResize []v1alpha1.PVCResizePolicy

	// True if the instance status is changed during the reconcile
	Dirty bool
}

//+kubebuilder:rbac:namespace=system,groups="",resources=pvcs,verbs=get;list;update;watch
//+kubebuilder:rbac:namespace=system,groups="",resources=pvs,verbs=get;list;
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

	err = r.initialize(ctx, req, logger, pvcResize)
	if err != nil {
		return r.handleError(err)
	}

	err = r.loadReferencedObjects()
	if err != nil {
		return r.handleError(err)
	}

	r.checkPoliciesToResize()
	if r.anythingToResize() {
		r.State.PVCResize.Status.Phase = "Resizing"
		r.State.Dirty = true

		err = r.resizePVC()
		if err != nil {
			return r.handleError(err)
		}
	} else if r.State.PVCResize.Status.Phase != "Completed" {
		err = r.loadPVs()
		if err != nil {
			return r.handleError(err)
		}

		r.makeCompletedStatus()
		r.clearErrorCondition()
	}

	if r.State.Dirty {
		err = r.Client.Status().Update(r.State.Context, &r.State.PVCResize)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// For now Reconcile after 10s to check if the size changes at PVC, anyway
	return ctrl.Result{RequeueAfter: time.Second * 10}, nil
}

func (r *PVCResizeReconciler) initialize(ctx context.Context, req ctrl.Request, logger logr.Logger, pvcResize v1alpha1.PVCResize) error {
	r.State = State{
		ResourceName: req.NamespacedName,
		Context:      ctx,
		Logger:       logger,
		PVCResize:    pvcResize,
	}

	return nil
}

func (r *PVCResizeReconciler) loadReferencedObjects() error {
	r.State.PVCs = map[string]corev1.PersistentVolumeClaim{}

	for _, policy := range r.State.PVCResize.Spec.Policies {
		var pvc corev1.PersistentVolumeClaim

		pvcKey := client.ObjectKey{Name: policy.Ref, Namespace: r.State.ResourceName.Namespace}
		err := r.Client.Get(r.State.Context, pvcKey, &pvc)
		if err != nil {
			return err
		}

		r.State.PVCs[policy.Ref] = pvc
	}

	return nil
}

func (r *PVCResizeReconciler) resizePVC() error {
	for _, policy := range r.State.PoliciesToResize {
		pvc := r.State.PVCs[policy.Ref]

		newSize := resource.MustParse(policy.Size)
		// update the pvc only if size changes
		if !pvc.Spec.Resources.Requests[corev1.ResourceStorage].Equal(newSize) {
			pvc.Spec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse(policy.Size)

			// update pvc
			err := r.Client.Update(r.State.Context, &pvc)
			if err != nil {
				return err
			}

			r.updateResizingStatus(policy)
		}
	}

	return nil
}

func (r *PVCResizeReconciler) loadPVs() error {
	r.State.PVs = map[string]corev1.PersistentVolume{}
	for _, pvc := range r.State.PVCs {
		var pv corev1.PersistentVolume

		pvKey := client.ObjectKey{Name: pvc.Spec.VolumeName, Namespace: r.State.ResourceName.Namespace}
		err := r.Client.Get(r.State.Context, pvKey, &pv)
		if err != nil {
			return err
		}
		r.State.PVs[pvc.Name] = pv
	}

	return nil
}

func (r *PVCResizeReconciler) makeCompletedStatus() {
	resizeInProgress := slices.ContainsFunc(r.State.PVCResize.Spec.Policies, func(policy v1alpha1.PVCResizePolicy) bool {
		pv := r.State.PVs[policy.Ref]

		newSize := resource.MustParse(policy.Size)
		// update the pvc only if size changes
		if !pv.Spec.Capacity.Storage().Equal(newSize) {
			return true
		}

		return false
	})

	if !resizeInProgress {
		r.State.Dirty = true
		r.State.PVCResize.Status.Phase = "Completed"
		r.State.PVCResize.Status.PVCs = utils.MapFunc(r.State.PVCResize.Spec.Policies, func(policy v1alpha1.PVCResizePolicy) v1alpha1.ResizeStatus {
			return v1alpha1.ResizeStatus{
				PVCName:      policy.Ref,
				PVName:       r.State.PVCs[policy.Ref].Spec.VolumeName,
				ResizeStatus: "ResizingDone",
				Size:         policy.Size,
			}
		})

		r.State.PVCResize.Status.PVCNames = utils.MapFunc(r.State.PVCResize.Status.PVCs, func(rs v1alpha1.ResizeStatus) string {
			return rs.PVCName
		})

		r.State.PVCResize.Status.PVNames = utils.MapFunc(r.State.PVCResize.Status.PVCs, func(rs v1alpha1.ResizeStatus) string {
			return rs.PVName
		})

	}

}

func (r *PVCResizeReconciler) updateResizingStatus(policy v1alpha1.PVCResizePolicy) {
	i := slices.IndexFunc(r.State.PVCResize.Status.PVCs, func(pvc v1alpha1.ResizeStatus) bool {
		return pvc.PVCName == policy.Ref
	})

	if i > -1 {
		r.State.PVCResize.Status.PVCs[i].ResizeStatus = "Resizing"
		r.State.PVCResize.Status.PVCs[i].Size = policy.Size
	} else {
		r.State.PVCResize.Status.PVCs = append(r.State.PVCResize.Status.PVCs, v1alpha1.ResizeStatus{
			PVCName:      policy.Ref,
			PVName:       r.State.PVCs[policy.Ref].Spec.VolumeName,
			ResizeStatus: "Resizing",
			Size:         policy.Size,
		})
	}
}

func (r *PVCResizeReconciler) checkPoliciesToResize() error {
	for _, policy := range r.State.PVCResize.Spec.Policies {
		pvc := r.State.PVCs[policy.Ref]

		newSize := resource.MustParse(policy.Size)
		// update the pvc only if size changes
		if !pvc.Spec.Resources.Requests[corev1.ResourceStorage].Equal(newSize) {
			r.State.PoliciesToResize = append(r.State.PoliciesToResize, policy)
		}
	}

	return nil
}

func (r *PVCResizeReconciler) anythingToResize() bool {
	return len(r.State.PoliciesToResize) > 0
}

func (r *PVCResizeReconciler) handleError(err error) (ctrl.Result, error) {

	code, msg := extractError(err)

	condition := metav1.Condition{
		Type:               "Error",
		Status:             metav1.ConditionTrue,
		Reason:             code,
		Message:            msg,
		LastTransitionTime: metav1.NewTime(time.Now()),
	}

	// if condition already not there, set it there
	if !IsStatusConditionPresentAndEqual(r.State.PVCResize.Status.Conditions, "Error", metav1.ConditionTrue) {
		r.State.PVCResize.Status.Conditions = append(r.State.PVCResize.Status.Conditions, condition)

		err = r.Client.Status().Update(r.State.Context, &r.State.PVCResize)
	}

	return ctrl.Result{}, err

}

func (r *PVCResizeReconciler) clearErrorCondition() {
	for i, condition := range r.State.PVCResize.Status.Conditions {
		if condition.Type == "Error" && condition.Status == metav1.ConditionTrue {
			r.State.PVCResize.Status.Conditions[i].Message = ""
			r.State.PVCResize.Status.Conditions[i].Status = metav1.ConditionFalse

			r.State.Dirty = true
		}
	}
}

// IsStatusConditionPresentAndEqual returns true when conditionType is present and equal to status.
func IsStatusConditionPresentAndEqual(conditions []metav1.Condition, conditionType string, status metav1.ConditionStatus) bool {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return condition.Status == status
		}
	}

	return false
}

func extractError(err error) (string, string) {
	var se *k8serrors.StatusError

	if !errors.As(err, &se) {
		return "Unknown", err.Error()
	}

	// Try to extract a reason
	reason := k8serrors.ReasonForError(err)
	if reason == metav1.StatusReasonUnknown {
		return "Unknown", se.ErrStatus.Message
	}

	return fmt.Sprintf("K8s:%s", string(reason)), se.Error()
}

// SetupWithManager sets up the controller with the Manager.
func (r *PVCResizeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.PVCResize{}).
		Complete(r)
}
