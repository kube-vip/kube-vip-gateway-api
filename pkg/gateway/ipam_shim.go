package gateway

import (
	"context"
	"fmt"
	"net/netip"
	"strings"

	"github.com/kube-vip/kube-vip-cloud-provider/pkg/ipam"
	"go4.org/netipx"
	"sigs.k8s.io/gateway-api/apis/v1beta1"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *GatewayReconciler) retrieveIPAddress(ctx context.Context, namespace, configmapName string) (string, error) {
	// Retrieve the configmap that contains our IPAM configuration
	var configMap v1.ConfigMap
	key := types.NamespacedName{
		Namespace: namespace,
		Name:      configmapName,
	}
	err := r.Get(ctx, key, &configMap)
	if err != nil {
		return "", err
	}

	// Get ip pool from configmap and determine if it is namespace specific or global
	pool, _, err := discoverPool(&configMap, namespace, configmapName)
	if err != nil {
		return "", err
	}

	// Get all services in this namespace, that have the correct label
	var services v1.ServiceList
	selector := labels.SelectorFromSet(map[string]string{
		"implementation": r.ImplementationLabel,
	})

	err = r.List(ctx, &services, &client.ListOptions{LabelSelector: selector})
	if err != nil {
		return "", err
	}
	// Get all gatewats in this namespace, that have the correct label
	var gateways v1beta1.GatewayList

	err = r.List(ctx, &gateways, &client.ListOptions{LabelSelector: selector})
	if err != nil {
		return "", err
	}
	builder := &netipx.IPSetBuilder{}
	for x := range gateways.Items {
		addr, err := netip.ParseAddr(gateways.Items[x].Labels["ipam-address"])
		if err != nil {
			return "", err
		}
		builder.Add(addr)
	}
	inUseSet, err := builder.IPSet()
	if err != nil {
		return "", err
	}

	// If the LoadBalancer address is empty, then do a local IPAM lookup
	loadBalancerIP, err := discoverAddress(namespace, pool, inUseSet)

	if err != nil {
		return "", err
	}

	return loadBalancerIP, nil
}

func UniqueAddresses(addresses ...[]string) []string {
	uniqueMap := map[string]bool{}

	for _, intSlice := range addresses {
		for _, number := range intSlice {
			uniqueMap[number] = true
		}
	}

	// Create a slice with the capacity of unique items
	// This capacity make appending flow much more efficient
	result := make([]string, 0, len(uniqueMap))

	for key := range uniqueMap {
		result = append(result, key)
	}

	return result
}

func discoverPool(cm *v1.ConfigMap, namespace, configMapName string) (pool string, global bool, err error) {
	var cidr, ipRange string
	var ok bool

	// Find Cidr
	cidrKey := fmt.Sprintf("cidr-%s", namespace)
	// Lookup current namespace
	if cidr, ok = cm.Data[cidrKey]; !ok {
		klog.Info(fmt.Errorf("no cidr config for namespace [%s] exists in key [%s] configmap [%s]", namespace, cidrKey, configMapName))
		// Lookup global cidr configmap data
		if cidr, ok = cm.Data["cidr-global"]; !ok {
			klog.Info(fmt.Errorf("no global cidr config exists [cidr-global]"))
		} else {
			klog.Infof("Taking address from [cidr-global] pool")
			return cidr, true, nil
		}
	} else {
		klog.Infof("Taking address from [%s] pool", cidrKey)
		return cidr, false, nil
	}

	// Find Range
	rangeKey := fmt.Sprintf("range-%s", namespace)
	// Lookup current namespace
	if ipRange, ok = cm.Data[rangeKey]; !ok {
		klog.Info(fmt.Errorf("no range config for namespace [%s] exists in key [%s] configmap [%s]", namespace, rangeKey, configMapName))
		// Lookup global range configmap data
		if ipRange, ok = cm.Data["range-global"]; !ok {
			klog.Info(fmt.Errorf("no global range config exists [range-global]"))
		} else {
			klog.Infof("Taking address from [range-global] pool")
			return ipRange, true, nil
		}
	} else {
		klog.Infof("Taking address from [%s] pool", rangeKey)
		return ipRange, false, nil
	}

	return "", false, fmt.Errorf("no address pools could be found")
}

func discoverAddress(namespace, pool string, inUseIPSet *netipx.IPSet) (vip string, err error) {
	// Check if DHCP is required
	if pool == "0.0.0.0/32" {
		vip = "0.0.0.0"
		// Check if ip pool contains a cidr, if not assume it is a range
	} else if strings.Contains(pool, "/") {
		vip, err = ipam.FindAvailableHostFromCidr(namespace, pool, inUseIPSet)
		if err != nil {
			return "", err
		}
	} else {
		vip, err = ipam.FindAvailableHostFromRange(namespace, pool, inUseIPSet)
		if err != nil {
			return "", err
		}
	}

	return vip, err
}
