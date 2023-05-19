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
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
)

// UDPRouteReconciler reconciles a Cluster object
type UDPRouteReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	ControllerName      string
	ServiceBehaviour    string
	ImplementationLabel string
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
func (r *UDPRouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	var UDPRoute v1alpha2.UDPRoute
	if err := r.Get(ctx, req.NamespacedName, &UDPRoute); err != nil {
		if errors.IsNotFound(err) {
			// This will attempt to reconcile the services by deleting the service attached to this TCP Route
			return r.deleteService(ctx, req)
		}
		log.Error(err, "unable to fetch UDPRoute object")
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	finalizer := fmt.Sprintf("%s/finalizer", r.ControllerName)

	if UDPRoute.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(&UDPRoute, finalizer) {
			controllerutil.AddFinalizer(&UDPRoute, finalizer)
			if err := r.Update(ctx, &UDPRoute); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(&UDPRoute, finalizer) {
			// our finalizer is present, so lets handle any external dependency
			_, err := r.deleteService(ctx, req)
			if err != nil {
				// if fail to delete the external dependency here, return with error
				// so that it can be retried
				return ctrl.Result{}, err
			}

			// remove our finalizer from the list and update it.
			controllerutil.RemoveFinalizer(&UDPRoute, finalizer)
			if err := r.Update(ctx, &UDPRoute); err != nil {
				return ctrl.Result{}, err
			}
		}

		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil
	}

	// Find all parent resources (more than likely just one but YOLO)
	for x := range UDPRoute.Spec.ParentRefs {
		// Namespace logic!
		var gatewayNamespace string
		if UDPRoute.Spec.ParentRefs[x].Namespace != nil {
			gatewayNamespace = string(*UDPRoute.Spec.ParentRefs[x].Namespace)
		} else {
			gatewayNamespace = UDPRoute.Namespace
		}

		// Find the parent gateway!
		key := types.NamespacedName{
			Namespace: gatewayNamespace,
			Name:      string(UDPRoute.Spec.ParentRefs[x].Name),
		}

		gateway := &v1beta1.Gateway{}
		err := r.Client.Get(ctx, key, gateway, nil)
		if err != nil {
			return ctrl.Result{}, err
			//log.Error(err, fmt.Sprintf("Unknown Gateway [%v]", key.String()))
		} else {
			if len(gateway.Status.Addresses) == 0 {
				return ctrl.Result{RequeueAfter: time.Second * 10}, fmt.Errorf("gateway [%s], has no addresses assigned", gateway.Name)
			}
			// Find our listener
			if UDPRoute.Spec.ParentRefs[x].SectionName != nil {
				listener := &v1beta1.Listener{}
				for x := range gateway.Spec.Listeners {
					if gateway.Spec.Listeners[x].Name == *UDPRoute.Spec.ParentRefs[x].SectionName {
						listener = &gateway.Spec.Listeners[x]
					}
				}
				if listener != nil {
					// We've found our listener!
					// At this point we have our entrypoint

					// Now to parse our backends  ¯\_(ツ)_/¯
					for y := range UDPRoute.Spec.Rules {
						for z := range UDPRoute.Spec.Rules[y].BackendRefs {
							// Namespace logic!
							var serviceNamespace string
							if UDPRoute.Spec.Rules[y].BackendRefs[z].Namespace != nil {
								serviceNamespace = string(*UDPRoute.Spec.ParentRefs[x].Namespace)
							} else {
								serviceNamespace = UDPRoute.Namespace
							}
							err = r.reconcileService(ctx, string(UDPRoute.Spec.Rules[y].BackendRefs[z].Name), serviceNamespace, UDPRoute.Name, gateway.Spec.Addresses[0].Value, int(listener.Port), int(*UDPRoute.Spec.Rules[y].BackendRefs[z].Port), UDPRoute.Labels)
							if err != nil {
								return ctrl.Result{}, err
							}
						}

					}

				} else {
					log.Info(fmt.Sprintf("Unknown Listener on gateway [%s]", *UDPRoute.Spec.ParentRefs[x].SectionName))
				}
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *UDPRouteReconciler) deleteService(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// We will get ALL services in the namespace
	var services v1.ServiceList
	err := r.List(ctx, &services, &client.ListOptions{Namespace: req.Namespace})
	if err != nil {
		return ctrl.Result{}, err
	}
	for x := range services.Items {
		// Find out if we manage this item AND it references this UDPRoute object
		if services.Items[x].Annotations["gateway-api-controller"] == r.ControllerName && services.Items[x].Annotations["parent-tcp-route"] == req.Name {
			err = r.Delete(ctx, &services.Items[x], &client.DeleteOptions{})
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	}
	return ctrl.Result{}, nil
}

func (r *UDPRouteReconciler) reconcileService(ctx context.Context, name, namespace, parentName, address string, port, targetport int, selector map[string]string) error {
	// Set our behaviour for services
	var servicebehaviour = selector[serviceBehaviour]
	if servicebehaviour == "" { // If blank default to controller behaviour
		servicebehaviour = r.ServiceBehaviour
	}

	// does our service exist?
	var service v1.Service
	key := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
	err := r.Get(ctx, key, &service)

	// If there is an error, what we do with it will depend on the service behaviour
	switch servicebehaviour {
	case ServiceCreate:
		if err == nil {
			return fmt.Errorf("unable to create service [%s], as it already exists", name)
		}
		// Service doesn't exist (this error is a good thing)
		if errors.IsNotFound(err) {

			// This is the design of the service
			service.Name = name
			service.Namespace = namespace
			service.Annotations = map[string]string{}
			service.Annotations["gateway-api-controller"] = r.ControllerName
			service.Annotations["parent-tcp-route"] = parentName
			service.Labels = map[string]string{}
			service.Labels["ipam-address"] = address
			service.Labels["implementation"] = r.ImplementationLabel
			service.Spec.Type = v1.ServiceTypeLoadBalancer
			service.Spec.LoadBalancerIP = address
			service.Spec.Ports = []v1.ServicePort{
				{
					TargetPort: intstr.FromInt(targetport),
					Port:       int32(port),
					Protocol:   v1.ProtocolUDP,
				},
			}

			// Populate the selector from the labels
			if selector != nil {
				fmt.Print(selector)
				k := selector["selectorkey"]
				v := selector["selectorvalue"]
				if service.Spec.Selector == nil {
					service.Spec.Selector = map[string]string{}
				}
				service.Spec.Selector[k] = v
			}
			err = r.Create(ctx, &service, &client.CreateOptions{})
			if err != nil {
				return err
			}
		}
	case ServiceDuplicate:
		// No error means that the service exists.. lets copy it and create our own
		if err == nil {
			newService := service.DeepCopy()
			// Create our service
			newService.Name = name + "-gw-api"
			key.Name = newService.Name
			err = r.Get(ctx, key, newService)
			if errors.IsNotFound(err) {
				newService.Namespace = namespace
				newService.ResourceVersion = ""
				newService.Spec.ClusterIP = ""
				newService.Spec.ClusterIPs = []string{}
				// Initialise the Annotations
				newService.Annotations = map[string]string{}
				newService.Annotations["gateway-api-controller"] = r.ControllerName
				newService.Annotations["parent-tcp-route"] = parentName
				// Initialise the Labels
				if service.Labels == nil {
					service.Labels = map[string]string{}
				}
				service.Labels["ipam-address"] = address
				service.Labels["implementation"] = r.ImplementationLabel
				// Set service configuration
				newService.Spec.Type = v1.ServiceTypeLoadBalancer
				newService.Spec.Ports = []v1.ServicePort{
					{
						TargetPort: intstr.FromInt(targetport),
						Port:       int32(port),
						Protocol:   v1.ProtocolUDP,
					},
				}
				err = r.Create(ctx, newService, &client.CreateOptions{})
				if err != nil {
					return err
				}
			}
			if err != nil {
				return err
			}

		}
	case ServiceUpdate:
		if err == nil {
			service.Annotations["gateway-api-controller"] = r.ControllerName
			service.Annotations["parent-tcp-route"] = parentName
			// Set service configuration
			service.Spec.Type = v1.ServiceTypeLoadBalancer
			service.Spec.LoadBalancerIP = address
			service.Spec.Ports = []v1.ServicePort{
				{
					TargetPort: intstr.FromInt(targetport),
					Port:       int32(port),
					Protocol:   v1.ProtocolUDP,
				},
			}
			if service.Labels == nil {
				service.Labels = map[string]string{}
			}
			service.Labels["ipam-address"] = address
			service.Labels["implementation"] = r.ImplementationLabel
			err = r.Update(ctx, &service, &client.UpdateOptions{})
			if err != nil {
				return err
			}
		} else {
			return err
		}
	default:
		return fmt.Errorf("unknown service action [%s]", r.ServiceBehaviour)
	}

	// Add finalizer to service

	finalizer := fmt.Sprintf("%s/finalizer", r.ControllerName)

	if service.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(&service, finalizer) {
			controllerutil.AddFinalizer(&service, finalizer)
			if err := r.Update(ctx, &service); err != nil {
				return err
			}
		}
	}
	// } else {
	// 	// The object is being deleted
	// 	if controllerutil.ContainsFinalizer(&service, finalizer) {
	// 		// our finalizer is present, so lets handle any external dependency
	// 		// _, err := r.deleteService(ctx, req)
	// 		// if err != nil {
	// 		// 	// if fail to delete the external dependency here, return with error
	// 		// 	// so that it can be retried
	// 		// 	return  err
	// 		// }

	// 		// remove our finalizer from the list and update it.
	// 		controllerutil.RemoveFinalizer(&service, finalizer)
	// 		if err := r.Update(ctx, &service); err != nil {
	// 			return err
	// 		}
	// 	}

	// 	// Stop reconciliation as the item is being deleted
	// 	//return ctrl.Result{}, nil
	// }

	// All gravy
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *UDPRouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	switch r.ServiceBehaviour {
	case ServiceCreate, ServiceDuplicate, ServiceUpdate:
	default:
		return fmt.Errorf("unknown service behaviour, options are [%s/%s/%s]", ServiceCreate, ServiceDuplicate, ServiceUpdate)
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.UDPRoute{}).
		Complete(r)
}
