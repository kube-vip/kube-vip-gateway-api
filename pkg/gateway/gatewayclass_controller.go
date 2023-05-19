/*
Copyright 2022 Dan Finneran.

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

package gateway

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
)

// GatewaClassReconciler reconciles a Cluster object
type GatewayClassReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	ControllerName string
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Cluster object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *GatewayClassReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	var gatewayClass v1beta1.GatewayClass
	if err := r.Get(ctx, req.NamespacedName, &gatewayClass); err != nil {
		if errors.IsNotFound(err) {
			// object not found, could have been deleted after
			// reconcile request, hence don't requeue
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch Gateway Classes")
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	// your logic here
	log.Info("Reconciling Cluster", "Cluster", gatewayClass.Name)

	// This is a quick overview of setting the condition of a controller... if it matches our controller name then
	// we set it to ready
	if gatewayClass.Spec.ControllerName == v1beta1.GatewayController(r.ControllerName) {
		for x := range gatewayClass.Status.Conditions {
			if gatewayClass.Status.Conditions[x].Type == "Accepted" {
				gatewayClass.Status.Conditions[x].Status = metav1.ConditionTrue
				gatewayClass.Status.Conditions[x].LastTransitionTime = metav1.Now()
			}
		}
		err := r.Client.Status().Update(context.TODO(), &gatewayClass, &client.SubResourceUpdateOptions{})
		if err != nil {
			log.Error(err, "unable to update gateway class")
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayClassReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1beta1.GatewayClass{}).
		Complete(r)
}
