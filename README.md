# Kube-vip Gateway API 

![](https://github.com/kube-vip/kube-vip/raw/main/kube-vip.png)

This is an initial implementation of the various controllers required in order to manage Gateway API network deployments within Kubernetes. At the moment four basic controllers are implemented:

- GatewayClass
- Gateway
- TCPRoute
- UDPRoute

## Usage

### Build

`go build` 

If I ever really learn how makefiles work, then perhaps i'll implement one

### Running

If you're running outside of a Kubernetes cluster then something like the following will work..

`./gateway-api-controller -metrics-bind-address :8083 -kubeconfig ~/.kube/config`

Want to change the gatewayClass then the flag `-gateway-class-name` will probably help, setting the `-ipam-configmap` will point to a configmap that contains the range or cidr used for IPAM. 

Create that range with the following:
```
kubectl create configmap --namespace default <configmap_name> --from-literal range-global=172.18.100.10-172.18.100.30
```

### Example

The `/manifests` folder contains the basics of the `GatewayClass`, `Gateway` and `TCPRoute` yaml structure..

## Implemented logic

- Currently the `GatewayClass` will set the status `ACCEPTED -> True` if the gateway controller matches the flag `-gateway-class-name` 
- When a `Gateway` is created it will verify that the parent `GatewayClass` exists.
- The `Gateway` will also perform IPAM and apply an address to the `.Spec.Address` and `.Status.Address` fields
- The `TCPRoute` will look up its parent `gateway` and confirm that the it's the correct reference, it will then find the listener (external IP address)... with the listener and TCPRoute routes it will then lookup the referenced service. 

### Services implementation (WIP)

When creating a TCPRoute you can apply the following labels:

```
metadata:
  labels:
    selectorkey: app
    selectorvalue: my-nginx
    serviceBehaviour: create
```

The selector key/value is used when creating/updating/duplicating a corresponding service, the serviceBehaviour determins what the TCPRoute controller will do when a new route is created. (The default behaviour is to create a new service, referenced by `rules.backendRefs.name`)

### Thoughts

As Gateway-API has no concept of selectors (to identify a range of pods or endpoints), it refers to a a service though the `[]rules.[]backendRefs.name` (multiple rules, with multiple backends) with a destination port and destination service (identified as `name`). With L2/L3 loadbalancers not touching the dataplane we rely in Kubernetes services (of type=`LoadBalancer`) to configure the `kube-proxy` so that the dataplane works, without selectors we can't create "enough" of a new service that will map to endpoints.. we can refer to an existing service (that a user has to create) however. 

So what are the options moving forward:

#### Idea 1

Add key/value labels to a TCPRoute that "emulate" the selector on a service, we can then create a brand new service of type=`LoadBalancer` with the external address from the `gateway` and the destination settings of the `TCPRoute`.

#### Idea 2 

We can ask a user to create a quick service (clusterIP etc..) that has the selectors in it, we can then duplicate that service with type=`LoadBalancer` and set the `.spec.LoadBalancerIP` from the `gateway` address along with the additional config from the `TCPRoute`.

#### Idea 3

We can `update` the refered service with the configuration from the `gateway/TCPRoute` so that it behaves as we're asking Gateway API.

It will create a new service based upon that referenced service with the type loadbalancer and away we go...

 that's it so far (clearly a long way to go)

## Want to Contribute?

Please and thankyou