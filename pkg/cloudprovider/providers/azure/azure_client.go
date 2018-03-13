/*
Copyright 2017 The Kubernetes Authors.

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

package azure

import (
	"context"
	"net/http"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/arm/compute"
	"github.com/Azure/azure-sdk-for-go/arm/disk"
	"github.com/Azure/azure-sdk-for-go/arm/network"
	"github.com/Azure/azure-sdk-for-go/arm/storage"
	computepreview "github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2017-12-01/compute"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/golang/glog"

	"k8s.io/client-go/util/flowcontrol"
)

// Creates an error for rate limiting errors
func createArmRateLimitErr(isWrite bool, opName string) error {
	opType := "read"
	if isWrite {
		opType = "write"
	}
	return fmt.Errorf("azure - ARM rate limited(%s) for operation:%s", opType, opName)
}

// VirtualMachinesClient defines needed functions for azure compute.VirtualMachinesClient
type VirtualMachinesClient interface {
	CreateOrUpdate(resourceGroupName string, VMName string, parameters compute.VirtualMachine, cancel <-chan struct{}) (<-chan compute.VirtualMachine, <-chan error)
	Get(resourceGroupName string, VMName string, expand compute.InstanceViewTypes) (result compute.VirtualMachine, err error)
	List(resourceGroupName string) (result compute.VirtualMachineListResult, err error)
	ListNextResults(resourceGroupName string, lastResults compute.VirtualMachineListResult) (result compute.VirtualMachineListResult, err error)
}

// InterfacesClient defines needed functions for azure network.InterfacesClient
type InterfacesClient interface {
	CreateOrUpdate(resourceGroupName string, networkInterfaceName string, parameters network.Interface, cancel <-chan struct{}) (<-chan network.Interface, <-chan error)
	Get(resourceGroupName string, networkInterfaceName string, expand string) (result network.Interface, err error)
	GetVirtualMachineScaleSetNetworkInterface(resourceGroupName string, virtualMachineScaleSetName string, virtualmachineIndex string, networkInterfaceName string, expand string) (result network.Interface, err error)
}

// LoadBalancersClient defines needed functions for azure network.LoadBalancersClient
type LoadBalancersClient interface {
	CreateOrUpdate(resourceGroupName string, loadBalancerName string, parameters network.LoadBalancer, cancel <-chan struct{}) (<-chan network.LoadBalancer, <-chan error)
	Delete(resourceGroupName string, loadBalancerName string, cancel <-chan struct{}) (<-chan autorest.Response, <-chan error)
	Get(resourceGroupName string, loadBalancerName string, expand string) (result network.LoadBalancer, err error)
	List(resourceGroupName string) (result network.LoadBalancerListResult, err error)
	ListNextResults(resourceGroupName string, lastResult network.LoadBalancerListResult) (result network.LoadBalancerListResult, err error)
}

// PublicIPAddressesClient defines needed functions for azure network.PublicIPAddressesClient
type PublicIPAddressesClient interface {
	CreateOrUpdate(resourceGroupName string, publicIPAddressName string, parameters network.PublicIPAddress, cancel <-chan struct{}) (<-chan network.PublicIPAddress, <-chan error)
	Delete(resourceGroupName string, publicIPAddressName string, cancel <-chan struct{}) (<-chan autorest.Response, <-chan error)
	Get(resourceGroupName string, publicIPAddressName string, expand string) (result network.PublicIPAddress, err error)
	List(resourceGroupName string) (result network.PublicIPAddressListResult, err error)
	ListNextResults(resourceGroupName string, lastResults network.PublicIPAddressListResult) (result network.PublicIPAddressListResult, err error)
}

// SubnetsClient defines needed functions for azure network.SubnetsClient
type SubnetsClient interface {
	CreateOrUpdate(resourceGroupName string, virtualNetworkName string, subnetName string, subnetParameters network.Subnet, cancel <-chan struct{}) (<-chan network.Subnet, <-chan error)
	Delete(resourceGroupName string, virtualNetworkName string, subnetName string, cancel <-chan struct{}) (<-chan autorest.Response, <-chan error)
	Get(resourceGroupName string, virtualNetworkName string, subnetName string, expand string) (result network.Subnet, err error)
	List(resourceGroupName string, virtualNetworkName string) (result network.SubnetListResult, err error)
}

// SecurityGroupsClient defines needed functions for azure network.SecurityGroupsClient
type SecurityGroupsClient interface {
	CreateOrUpdate(resourceGroupName string, networkSecurityGroupName string, parameters network.SecurityGroup, cancel <-chan struct{}) (<-chan network.SecurityGroup, <-chan error)
	Delete(resourceGroupName string, networkSecurityGroupName string, cancel <-chan struct{}) (<-chan autorest.Response, <-chan error)
	Get(resourceGroupName string, networkSecurityGroupName string, expand string) (result network.SecurityGroup, err error)
	List(resourceGroupName string) (result network.SecurityGroupListResult, err error)
}

// VirtualMachineScaleSetsClient defines needed functions for azure computepreview.VirtualMachineScaleSetsClient
type VirtualMachineScaleSetsClient interface {
	CreateOrUpdate(ctx context.Context, resourceGroupName string, VMScaleSetName string, parameters computepreview.VirtualMachineScaleSet) (resp *http.Response, err error)
	Get(ctx context.Context, resourceGroupName string, VMScaleSetName string) (result computepreview.VirtualMachineScaleSet, err error)
	List(ctx context.Context, resourceGroupName string) (result []computepreview.VirtualMachineScaleSet, err error)
	UpdateInstances(ctx context.Context, resourceGroupName string, VMScaleSetName string, VMInstanceIDs computepreview.VirtualMachineScaleSetVMInstanceRequiredIDs) (resp *http.Response, err error)
}

// VirtualMachineScaleSetVMsClient defines needed functions for azure computepreview.VirtualMachineScaleSetVMsClient
type VirtualMachineScaleSetVMsClient interface {
	Get(ctx context.Context, resourceGroupName string, VMScaleSetName string, instanceID string) (result computepreview.VirtualMachineScaleSetVM, err error)
	GetInstanceView(ctx context.Context, resourceGroupName string, VMScaleSetName string, instanceID string) (result computepreview.VirtualMachineScaleSetVMInstanceView, err error)
	List(ctx context.Context, resourceGroupName string, virtualMachineScaleSetName string, filter string, selectParameter string, expand string) (result []computepreview.VirtualMachineScaleSetVM, err error)
	Update(ctx context.Context, resourceGroupName string, VMScaleSetName string, instanceID string, parameters computepreview.VirtualMachineScaleSetVM) (resp *http.Response, err error)
}

// RoutesClient defines needed functions for azure network.RoutesClient
type RoutesClient interface {
	CreateOrUpdate(resourceGroupName string, routeTableName string, routeName string, routeParameters network.Route, cancel <-chan struct{}) (<-chan network.Route, <-chan error)
	Delete(resourceGroupName string, routeTableName string, routeName string, cancel <-chan struct{}) (<-chan autorest.Response, <-chan error)
}

// RouteTablesClient defines needed functions for azure network.RouteTablesClient
type RouteTablesClient interface {
	CreateOrUpdate(resourceGroupName string, routeTableName string, parameters network.RouteTable, cancel <-chan struct{}) (<-chan network.RouteTable, <-chan error)
	Get(resourceGroupName string, routeTableName string, expand string) (result network.RouteTable, err error)
}

// StorageAccountClient defines needed functions for azure storage.AccountsClient
type StorageAccountClient interface {
	Create(resourceGroupName string, accountName string, parameters storage.AccountCreateParameters, cancel <-chan struct{}) (<-chan storage.Account, <-chan error)
	Delete(resourceGroupName string, accountName string) (result autorest.Response, err error)
	ListKeys(resourceGroupName string, accountName string) (result storage.AccountListKeysResult, err error)
	ListByResourceGroup(resourceGroupName string) (result storage.AccountListResult, err error)
	GetProperties(resourceGroupName string, accountName string) (result storage.Account, err error)
}

// DisksClient defines needed functions for azure disk.DisksClient
type DisksClient interface {
	CreateOrUpdate(resourceGroupName string, diskName string, diskParameter disk.Model, cancel <-chan struct{}) (<-chan disk.Model, <-chan error)
	Delete(resourceGroupName string, diskName string, cancel <-chan struct{}) (<-chan disk.OperationStatusResponse, <-chan error)
	Get(resourceGroupName string, diskName string) (result disk.Model, err error)
}

// azClientConfig contains all essential information to create an Azure client.
type azClientConfig struct {
	subscriptionID          string
	resourceManagerEndpoint string
	servicePrincipalToken   *adal.ServicePrincipalToken
	// ARM Rate limiting for GET vs PUT/POST
	//Details: https://docs.microsoft.com/en-us/azure/azure-resource-manager/resource-manager-request-limits
	rateLimiterReader flowcontrol.RateLimiter
	rateLimiterWriter flowcontrol.RateLimiter
}

// azVirtualMachinesClient implements VirtualMachinesClient.
type azVirtualMachinesClient struct {
	client            compute.VirtualMachinesClient
	rateLimiterReader flowcontrol.RateLimiter
	rateLimiterWriter flowcontrol.RateLimiter
}

func getContextWithCancel() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

func newAzVirtualMachinesClient(config *azClientConfig) *azVirtualMachinesClient {
	virtualMachinesClient := compute.NewVirtualMachinesClient(config.subscriptionID)
	virtualMachinesClient.BaseURI = config.resourceManagerEndpoint
	virtualMachinesClient.Authorizer = autorest.NewBearerAuthorizer(config.servicePrincipalToken)
	virtualMachinesClient.PollingDelay = 5 * time.Second
	configureUserAgent(&virtualMachinesClient.Client)

	return &azVirtualMachinesClient{
		rateLimiterReader: config.rateLimiterReader,
		rateLimiterWriter: config.rateLimiterWriter,
		client:            virtualMachinesClient,
	}
}

func (az *azVirtualMachinesClient) CreateOrUpdate(resourceGroupName string, VMName string, parameters compute.VirtualMachine, cancel <-chan struct{}) (<-chan compute.VirtualMachine, <-chan error) {
	/* Write rate limiting */
	if !az.rateLimiterWriter.TryAccept() {
		err := createArmRateLimitErr(true, "NSGCreateOrUpdate")
		errChan := make(chan error, 1)
		resultChan := make(chan compute.VirtualMachine, 1)
		errChan <- err
		resultChan <- compute.VirtualMachine{}
		return resultChan, errChan
	}

	glog.V(10).Infof("azVirtualMachinesClient.CreateOrUpdate(%q, %q): start", resourceGroupName, VMName)
	defer func() {
		glog.V(10).Infof("azVirtualMachinesClient.CreateOrUpdate(%q, %q): end", resourceGroupName, VMName)
	}()

	errChan := make(chan error, 1)
	mc := newMetricContext("vm", "create_or_update", resourceGroupName, az.client.SubscriptionID)
	resultChan, proxyErrChan := az.client.CreateOrUpdate(resourceGroupName, VMName, parameters, cancel)
	err := <-proxyErrChan
	mc.Observe(err)
	errChan <- err
	return resultChan, errChan
}

func (az *azVirtualMachinesClient) Get(resourceGroupName string, VMName string, expand compute.InstanceViewTypes) (result compute.VirtualMachine, err error) {
	err = createArmRateLimitErr(false, "VMGet")
	if !az.rateLimiterReader.TryAccept() {
		return
	}

	glog.V(10).Infof("azVirtualMachinesClient.Get(%q, %q): start", resourceGroupName, VMName)
	defer func() {
		glog.V(10).Infof("azVirtualMachinesClient.Get(%q, %q): end", resourceGroupName, VMName)
	}()

	mc := newMetricContext("vm", "get", resourceGroupName, az.client.SubscriptionID)
	result, err = az.client.Get(resourceGroupName, VMName, expand)
	mc.Observe(err)
	return
}

func (az *azVirtualMachinesClient) List(resourceGroupName string) (result compute.VirtualMachineListResult, err error) {
	err = createArmRateLimitErr(false, "VMList")
	if !az.rateLimiterReader.TryAccept() {
		return
	}

	glog.V(10).Infof("azVirtualMachinesClient.List(%q): start", resourceGroupName)
	defer func() {
		glog.V(10).Infof("azVirtualMachinesClient.List(%q): end", resourceGroupName)
	}()

	mc := newMetricContext("vm", "list", resourceGroupName, az.client.SubscriptionID)
	result, err = az.client.List(resourceGroupName)
	mc.Observe(err)
	return
}

func (az *azVirtualMachinesClient) ListNextResults(resourceGroupName string, lastResults compute.VirtualMachineListResult) (result compute.VirtualMachineListResult, err error) {
	err = createArmRateLimitErr(false, "VMListNextResults")
	if !az.rateLimiterReader.TryAccept() {
		return
	}

	glog.V(10).Infof("azVirtualMachinesClient.ListNextResults(%q): start", lastResults)
	defer func() {
		glog.V(10).Infof("azVirtualMachinesClient.ListNextResults(%q): end", lastResults)
	}()

	mc := newMetricContext("vm", "list_next_results", resourceGroupName, az.client.SubscriptionID)
	result, err = az.client.ListNextResults(lastResults)
	mc.Observe(err)
	return
}

// azInterfacesClient implements InterfacesClient.
type azInterfacesClient struct {
	client            network.InterfacesClient
	rateLimiterReader flowcontrol.RateLimiter
	rateLimiterWriter flowcontrol.RateLimiter
}

func newAzInterfacesClient(config *azClientConfig) *azInterfacesClient {
	interfacesClient := network.NewInterfacesClient(config.subscriptionID)
	interfacesClient.BaseURI = config.resourceManagerEndpoint
	interfacesClient.Authorizer = autorest.NewBearerAuthorizer(config.servicePrincipalToken)
	interfacesClient.PollingDelay = 5 * time.Second
	configureUserAgent(&interfacesClient.Client)

	return &azInterfacesClient{
		rateLimiterReader: config.rateLimiterReader,
		rateLimiterWriter: config.rateLimiterWriter,
		client:            interfacesClient,
	}
}

func (az *azInterfacesClient) CreateOrUpdate(resourceGroupName string, networkInterfaceName string, parameters network.Interface, cancel <-chan struct{}) (<-chan network.Interface, <-chan error) {

	/* Write rate limiting */
	if !az.rateLimiterWriter.TryAccept() {
		err := createArmRateLimitErr(true, "NiCreateOrUpdate")
		errChan := make(chan error, 1)
		resultChan := make(chan network.Interface, 1)
		errChan <- err
		resultChan <- network.Interface{}
		return resultChan, errChan
	}

	glog.V(10).Infof("azInterfacesClient.CreateOrUpdate(%q,%q): start", resourceGroupName, networkInterfaceName)
	defer func() {
		glog.V(10).Infof("azInterfacesClient.CreateOrUpdate(%q,%q): end", resourceGroupName, networkInterfaceName)
	}()

	errChan := make(chan error, 1)
	mc := newMetricContext("interfaces", "create_or_update", resourceGroupName, az.client.SubscriptionID)
	resultChan, proxyErrChan := az.client.CreateOrUpdate(resourceGroupName, networkInterfaceName, parameters, cancel)
	err := <-proxyErrChan
	mc.Observe(err)
	errChan <- err
	return resultChan, errChan
}

func (az *azInterfacesClient) Get(resourceGroupName string, networkInterfaceName string, expand string) (result network.Interface, err error) {
	err = createArmRateLimitErr(false, "NicGet")
	if !az.rateLimiterReader.TryAccept() {
		return
	}

	glog.V(10).Infof("azInterfacesClient.Get(%q,%q): start", resourceGroupName, networkInterfaceName)
	defer func() {
		glog.V(10).Infof("azInterfacesClient.Get(%q,%q): end", resourceGroupName, networkInterfaceName)
	}()

	mc := newMetricContext("interfaces", "get", resourceGroupName, az.client.SubscriptionID)
	result, err = az.client.Get(resourceGroupName, networkInterfaceName, expand)
	mc.Observe(err)
	return
}

func (az *azInterfacesClient) GetVirtualMachineScaleSetNetworkInterface(resourceGroupName string, virtualMachineScaleSetName string, virtualmachineIndex string, networkInterfaceName string, expand string) (result network.Interface, err error) {
	err = createArmRateLimitErr(false, "NicGetVirtualMachineScaleSetNetworkInterface")
	if !az.rateLimiterReader.TryAccept() {
		return
	}

	glog.V(10).Infof("azInterfacesClient.GetVirtualMachineScaleSetNetworkInterface(%q,%q,%q,%q): start", resourceGroupName, virtualMachineScaleSetName, virtualmachineIndex, networkInterfaceName)
	defer func() {
		glog.V(10).Infof("azInterfacesClient.GetVirtualMachineScaleSetNetworkInterface(%q,%q,%q,%q): end", resourceGroupName, virtualMachineScaleSetName, virtualmachineIndex, networkInterfaceName)
	}()

	mc := newMetricContext("interfaces", "get_vmss_ni", resourceGroupName, az.client.SubscriptionID)
	result, err = az.client.GetVirtualMachineScaleSetNetworkInterface(resourceGroupName, virtualMachineScaleSetName, virtualmachineIndex, networkInterfaceName, expand)
	mc.Observe(err)
	return
}

// azLoadBalancersClient implements LoadBalancersClient.
type azLoadBalancersClient struct {
	client            network.LoadBalancersClient
	rateLimiterReader flowcontrol.RateLimiter
	rateLimiterWriter flowcontrol.RateLimiter
}

func newAzLoadBalancersClient(config *azClientConfig) *azLoadBalancersClient {
	loadBalancerClient := network.NewLoadBalancersClient(config.subscriptionID)
	loadBalancerClient.BaseURI = config.resourceManagerEndpoint
	loadBalancerClient.Authorizer = autorest.NewBearerAuthorizer(config.servicePrincipalToken)
	loadBalancerClient.PollingDelay = 5 * time.Second
	configureUserAgent(&loadBalancerClient.Client)

	return &azLoadBalancersClient{
		rateLimiterReader: config.rateLimiterReader,
		rateLimiterWriter: config.rateLimiterWriter,
		client:            loadBalancerClient,
	}
}

func (az *azLoadBalancersClient) CreateOrUpdate(resourceGroupName string, loadBalancerName string, parameters network.LoadBalancer, cancel <-chan struct{}) (<-chan network.LoadBalancer, <-chan error) {
	/* Write rate limiting */
	if !az.rateLimiterWriter.TryAccept() {
		err := createArmRateLimitErr(true, "LBCreateOrUpdate")
		errChan := make(chan error, 1)
		resultChan := make(chan network.LoadBalancer, 1)
		errChan <- err
		resultChan <- network.LoadBalancer{}
		return resultChan, errChan
	}

	glog.V(10).Infof("azLoadBalancersClient.CreateOrUpdate(%q,%q): start", resourceGroupName, loadBalancerName)
	defer func() {
		glog.V(10).Infof("azLoadBalancersClient.CreateOrUpdate(%q,%q): end", resourceGroupName, loadBalancerName)
	}()

	errChan := make(chan error, 1)
	mc := newMetricContext("load_balancers", "create_or_update", resourceGroupName, az.client.SubscriptionID)
	resultChan, proxyErrChan := az.client.CreateOrUpdate(resourceGroupName, loadBalancerName, parameters, cancel)
	err := <-proxyErrChan
	mc.Observe(err)
	errChan <- err
	return resultChan, errChan
}

func (az *azLoadBalancersClient) Delete(resourceGroupName string, loadBalancerName string, cancel <-chan struct{}) (<-chan autorest.Response, <-chan error) {
	/* Write rate limiting */
	if !az.rateLimiterWriter.TryAccept() {
		err := createArmRateLimitErr(true, "LBDelete")
		errChan := make(chan error, 1)
		resultChan := make(chan autorest.Response, 1)
		errChan <- err
		resultChan <- autorest.Response{}
		return resultChan, errChan
	}

	glog.V(10).Infof("azLoadBalancersClient.Delete(%q,%q): start", resourceGroupName, loadBalancerName)
	defer func() {
		glog.V(10).Infof("azLoadBalancersClient.Delete(%q,%q): end", resourceGroupName, loadBalancerName)
	}()

	errChan := make(chan error, 1)
	mc := newMetricContext("load_balancers", "delete", resourceGroupName, az.client.SubscriptionID)
	resultChan, proxyErrChan := az.client.Delete(resourceGroupName, loadBalancerName, cancel)
	err := <-proxyErrChan
	mc.Observe(err)
	errChan <- err
	return resultChan, errChan
}

func (az *azLoadBalancersClient) Get(resourceGroupName string, loadBalancerName string, expand string) (result network.LoadBalancer, err error) {
	err = createArmRateLimitErr(false, "LBGet")
	if !az.rateLimiterReader.TryAccept() {
		return
	}

	glog.V(10).Infof("azLoadBalancersClient.Get(%q,%q): start", resourceGroupName, loadBalancerName)
	defer func() {
		glog.V(10).Infof("azLoadBalancersClient.Get(%q,%q): end", resourceGroupName, loadBalancerName)
	}()

	mc := newMetricContext("load_balancers", "get", resourceGroupName, az.client.SubscriptionID)
	result, err = az.client.Get(resourceGroupName, loadBalancerName, expand)
	mc.Observe(err)
	return
}

func (az *azLoadBalancersClient) List(resourceGroupName string) (result network.LoadBalancerListResult, err error) {
	err = createArmRateLimitErr(false, "LBList")
	if !az.rateLimiterReader.TryAccept() {
		return
	}

	glog.V(10).Infof("azLoadBalancersClient.List(%q): start", resourceGroupName)
	defer func() {
		glog.V(10).Infof("azLoadBalancersClient.List(%q): end", resourceGroupName)
	}()

	mc := newMetricContext("load_balancers", "list", resourceGroupName, az.client.SubscriptionID)
	result, err = az.client.List(resourceGroupName)
	mc.Observe(err)
	return
}

func (az *azLoadBalancersClient) ListNextResults(resourceGroupName string, lastResult network.LoadBalancerListResult) (result network.LoadBalancerListResult, err error) {
	err = createArmRateLimitErr(false, "LBListNextResults")
	if !az.rateLimiterReader.TryAccept() {
		return
	}

	glog.V(10).Infof("azLoadBalancersClient.ListNextResults(%q): start", lastResult)
	defer func() {
		glog.V(10).Infof("azLoadBalancersClient.ListNextResults(%q): end", lastResult)
	}()

	mc := newMetricContext("load_balancers", "list_next_results", resourceGroupName, az.client.SubscriptionID)
	result, err = az.client.ListNextResults(lastResult)
	mc.Observe(err)
	return
}

// azPublicIPAddressesClient implements PublicIPAddressesClient.
type azPublicIPAddressesClient struct {
	client            network.PublicIPAddressesClient
	rateLimiterReader flowcontrol.RateLimiter
	rateLimiterWriter flowcontrol.RateLimiter
}

func newAzPublicIPAddressesClient(config *azClientConfig) *azPublicIPAddressesClient {
	publicIPAddressClient := network.NewPublicIPAddressesClient(config.subscriptionID)
	publicIPAddressClient.BaseURI = config.resourceManagerEndpoint
	publicIPAddressClient.Authorizer = autorest.NewBearerAuthorizer(config.servicePrincipalToken)
	publicIPAddressClient.PollingDelay = 5 * time.Second
	configureUserAgent(&publicIPAddressClient.Client)

	return &azPublicIPAddressesClient{
		rateLimiterReader: config.rateLimiterReader,
		rateLimiterWriter: config.rateLimiterWriter,
		client:            publicIPAddressClient,
	}
}

func (az *azPublicIPAddressesClient) CreateOrUpdate(resourceGroupName string, publicIPAddressName string, parameters network.PublicIPAddress, cancel <-chan struct{}) (<-chan network.PublicIPAddress, <-chan error) {
	/* Write rate limiting */
	if !az.rateLimiterWriter.TryAccept() {
		err := createArmRateLimitErr(true, "PublicIPCreateOrUpdate")
		errChan := make(chan error, 1)
		resultChan := make(chan network.PublicIPAddress, 1)
		errChan <- err
		resultChan <- network.PublicIPAddress{}
		return resultChan, errChan
	}

	glog.V(10).Infof("azPublicIPAddressesClient.CreateOrUpdate(%q,%q): start", resourceGroupName, publicIPAddressName)
	defer func() {
		glog.V(10).Infof("azPublicIPAddressesClient.CreateOrUpdate(%q,%q): end", resourceGroupName, publicIPAddressName)
	}()

	errChan := make(chan error, 1)
	mc := newMetricContext("public_ip_addresses", "create_or_update", resourceGroupName, az.client.SubscriptionID)
	resultChan, proxyErrChan := az.client.CreateOrUpdate(resourceGroupName, publicIPAddressName, parameters, cancel)
	err := <-proxyErrChan
	mc.Observe(err)
	errChan <- err
	return resultChan, errChan
}

func (az *azPublicIPAddressesClient) Delete(resourceGroupName string, publicIPAddressName string, cancel <-chan struct{}) (<-chan autorest.Response, <-chan error) {
	/* Write rate limiting */
	if !az.rateLimiterWriter.TryAccept() {
		err := createArmRateLimitErr(true, "PublicIPDelete")
		errChan := make(chan error, 1)
		resultChan := make(chan autorest.Response, 1)
		errChan <- err
		resultChan <- autorest.Response{}
		return resultChan, errChan
	}

	glog.V(10).Infof("azPublicIPAddressesClient.Delete(%q,%q): start", resourceGroupName, publicIPAddressName)
	defer func() {
		glog.V(10).Infof("azPublicIPAddressesClient.Delete(%q,%q): end", resourceGroupName, publicIPAddressName)
	}()

	errChan := make(chan error, 1)
	mc := newMetricContext("public_ip_addresses", "delete", resourceGroupName, az.client.SubscriptionID)
	resultChan, proxyErrChan := az.client.Delete(resourceGroupName, publicIPAddressName, cancel)
	err := <-proxyErrChan
	mc.Observe(err)
	errChan <- err
	return resultChan, errChan
}

func (az *azPublicIPAddressesClient) Get(resourceGroupName string, publicIPAddressName string, expand string) (result network.PublicIPAddress, err error) {
	err = createArmRateLimitErr(false, "PublicIPGet")
	if !az.rateLimiterReader.TryAccept() {
		return
	}

	glog.V(10).Infof("azPublicIPAddressesClient.Get(%q,%q): start", resourceGroupName, publicIPAddressName)
	defer func() {
		glog.V(10).Infof("azPublicIPAddressesClient.Get(%q,%q): end", resourceGroupName, publicIPAddressName)
	}()

	mc := newMetricContext("public_ip_addresses", "get", resourceGroupName, az.client.SubscriptionID)
	result, err = az.client.Get(resourceGroupName, publicIPAddressName, expand)
	mc.Observe(err)
	return
}

func (az *azPublicIPAddressesClient) List(resourceGroupName string) (result network.PublicIPAddressListResult, err error) {
	err = createArmRateLimitErr(false, "PublicIPList")
	if !az.rateLimiterReader.TryAccept() {
		return
	}

	glog.V(10).Infof("azPublicIPAddressesClient.List(%q): start", resourceGroupName)
	defer func() {
		glog.V(10).Infof("azPublicIPAddressesClient.List(%q): end", resourceGroupName)
	}()

	mc := newMetricContext("public_ip_addresses", "list", resourceGroupName, az.client.SubscriptionID)
	result, err = az.client.List(resourceGroupName)
	mc.Observe(err)
	return
}

func (az *azPublicIPAddressesClient) ListNextResults(resourceGroupName string, lastResults network.PublicIPAddressListResult) (result network.PublicIPAddressListResult, err error) {
	err = createArmRateLimitErr(false, "PublicIPListNextResults")
	if !az.rateLimiterReader.TryAccept() {
		return
	}

	glog.V(10).Infof("azPublicIPAddressesClient.ListNextResults(%q): start", lastResults)
	defer func() {
		glog.V(10).Infof("azPublicIPAddressesClient.ListNextResults(%q): end", lastResults)
	}()

	mc := newMetricContext("public_ip_addresses", "list_next_results", resourceGroupName, az.client.SubscriptionID)
	result, err = az.client.ListNextResults(lastResults)
	mc.Observe(err)
	return
}

// azSubnetsClient implements SubnetsClient.
type azSubnetsClient struct {
	client            network.SubnetsClient
	rateLimiterReader flowcontrol.RateLimiter
	rateLimiterWriter flowcontrol.RateLimiter
}

func newAzSubnetsClient(config *azClientConfig) *azSubnetsClient {
	subnetsClient := network.NewSubnetsClient(config.subscriptionID)
	subnetsClient.BaseURI = config.resourceManagerEndpoint
	subnetsClient.Authorizer = autorest.NewBearerAuthorizer(config.servicePrincipalToken)
	subnetsClient.PollingDelay = 5 * time.Second
	configureUserAgent(&subnetsClient.Client)

	return &azSubnetsClient{
		client:            subnetsClient,
		rateLimiterReader: config.rateLimiterReader,
		rateLimiterWriter: config.rateLimiterWriter,
	}
}

func (az *azSubnetsClient) CreateOrUpdate(resourceGroupName string, virtualNetworkName string, subnetName string, subnetParameters network.Subnet, cancel <-chan struct{}) (<-chan network.Subnet, <-chan error) {
	/* Write rate limiting */
	if !az.rateLimiterWriter.TryAccept() {
		err := createArmRateLimitErr(true, "SubnetCreateOrUpdate")
		errChan := make(chan error, 1)
		resultChan := make(chan network.Subnet, 1)
		errChan <- err
		resultChan <- network.Subnet{}
		return resultChan, errChan
	}

	glog.V(10).Infof("azSubnetsClient.CreateOrUpdate(%q,%q,%q): start", resourceGroupName, virtualNetworkName, subnetName)
	defer func() {
		glog.V(10).Infof("azSubnetsClient.CreateOrUpdate(%q,%q,%q): end", resourceGroupName, virtualNetworkName, subnetName)
	}()

	errChan := make(chan error, 1)
	mc := newMetricContext("subnets", "create_or_update", resourceGroupName, az.client.SubscriptionID)
	resultChan, proxyErrChan := az.client.CreateOrUpdate(resourceGroupName, virtualNetworkName, subnetName, subnetParameters, cancel)
	err := <-proxyErrChan
	mc.Observe(err)
	errChan <- err
	return resultChan, errChan
}

func (az *azSubnetsClient) Delete(resourceGroupName string, virtualNetworkName string, subnetName string, cancel <-chan struct{}) (<-chan autorest.Response, <-chan error) {
	/* Write rate limiting */
	if !az.rateLimiterWriter.TryAccept() {
		err := createArmRateLimitErr(true, "SubnetDelete")
		errChan := make(chan error, 1)
		resultChan := make(chan autorest.Response, 1)
		errChan <- err
		resultChan <- autorest.Response{}
		return resultChan, errChan
	}

	glog.V(10).Infof("azSubnetsClient.Delete(%q,%q,%q): start", resourceGroupName, virtualNetworkName, subnetName)
	defer func() {
		glog.V(10).Infof("azSubnetsClient.Delete(%q,%q,%q): end", resourceGroupName, virtualNetworkName, subnetName)
	}()

	errChan := make(chan error, 1)
	mc := newMetricContext("subnets", "delete", resourceGroupName, az.client.SubscriptionID)
	resultChan, proxyErrChan := az.client.Delete(resourceGroupName, virtualNetworkName, subnetName, cancel)
	err := <-proxyErrChan
	mc.Observe(err)
	errChan <- err
	return resultChan, errChan
}

func (az *azSubnetsClient) Get(resourceGroupName string, virtualNetworkName string, subnetName string, expand string) (result network.Subnet, err error) {
	err = createArmRateLimitErr(false, "SubnetGet")
	if !az.rateLimiterReader.TryAccept() {
		return
	}

	glog.V(10).Infof("azSubnetsClient.Get(%q,%q,%q): start", resourceGroupName, virtualNetworkName, subnetName)
	defer func() {
		glog.V(10).Infof("azSubnetsClient.Get(%q,%q,%q): end", resourceGroupName, virtualNetworkName, subnetName)
	}()

	mc := newMetricContext("subnets", "get", resourceGroupName, az.client.SubscriptionID)
	result, err = az.client.Get(resourceGroupName, virtualNetworkName, subnetName, expand)
	mc.Observe(err)
	return
}

func (az *azSubnetsClient) List(resourceGroupName string, virtualNetworkName string) (result network.SubnetListResult, err error) {
	err = createArmRateLimitErr(false, "SubnetList")
	if !az.rateLimiterReader.TryAccept() {
		return
	}

	glog.V(10).Infof("azSubnetsClient.List(%q,%q): start", resourceGroupName, virtualNetworkName)
	defer func() {
		glog.V(10).Infof("azSubnetsClient.List(%q,%q): end", resourceGroupName, virtualNetworkName)
	}()

	mc := newMetricContext("subnets", "list", resourceGroupName, az.client.SubscriptionID)
	result, err = az.client.List(resourceGroupName, virtualNetworkName)
	mc.Observe(err)
	return
}

// azSecurityGroupsClient implements SecurityGroupsClient.
type azSecurityGroupsClient struct {
	client            network.SecurityGroupsClient
	rateLimiterReader flowcontrol.RateLimiter
	rateLimiterWriter flowcontrol.RateLimiter
}

func newAzSecurityGroupsClient(config *azClientConfig) *azSecurityGroupsClient {
	securityGroupsClient := network.NewSecurityGroupsClient(config.subscriptionID)
	securityGroupsClient.BaseURI = config.resourceManagerEndpoint
	securityGroupsClient.Authorizer = autorest.NewBearerAuthorizer(config.servicePrincipalToken)
	securityGroupsClient.PollingDelay = 5 * time.Second
	configureUserAgent(&securityGroupsClient.Client)

	return &azSecurityGroupsClient{
		client:            securityGroupsClient,
		rateLimiterReader: config.rateLimiterReader,
		rateLimiterWriter: config.rateLimiterWriter,
	}
}

func (az *azSecurityGroupsClient) CreateOrUpdate(resourceGroupName string, networkSecurityGroupName string, parameters network.SecurityGroup, cancel <-chan struct{}) (<-chan network.SecurityGroup, <-chan error) {
	/* Write rate limiting */
	if !az.rateLimiterWriter.TryAccept() {
		err := createArmRateLimitErr(true, "NSGCreateOrUpdate")
		errChan := make(chan error, 1)
		resultChan := make(chan network.SecurityGroup, 1)
		errChan <- err
		resultChan <- network.SecurityGroup{}
		return resultChan, errChan
	}

	glog.V(10).Infof("azSecurityGroupsClient.CreateOrUpdate(%q,%q): start", resourceGroupName, networkSecurityGroupName)
	defer func() {
		glog.V(10).Infof("azSecurityGroupsClient.CreateOrUpdate(%q,%q): end", resourceGroupName, networkSecurityGroupName)
	}()

	errChan := make(chan error, 1)
	mc := newMetricContext("security_groups", "create_or_update", resourceGroupName, az.client.SubscriptionID)
	resultChan, proxyErrChan := az.client.CreateOrUpdate(resourceGroupName, networkSecurityGroupName, parameters, cancel)
	err := <-proxyErrChan
	mc.Observe(err)
	errChan <- err
	return resultChan, errChan
}

func (az *azSecurityGroupsClient) Delete(resourceGroupName string, networkSecurityGroupName string, cancel <-chan struct{}) (<-chan autorest.Response, <-chan error) {
	/* Write rate limiting */
	if !az.rateLimiterWriter.TryAccept() {
		err := createArmRateLimitErr(true, "NSGDelete")
		errChan := make(chan error, 1)
		resultChan := make(chan autorest.Response, 1)
		errChan <- err
		resultChan <- autorest.Response{}
		return resultChan, errChan
	}

	glog.V(10).Infof("azSecurityGroupsClient.Delete(%q,%q): start", resourceGroupName, networkSecurityGroupName)
	defer func() {
		glog.V(10).Infof("azSecurityGroupsClient.Delete(%q,%q): end", resourceGroupName, networkSecurityGroupName)
	}()

	errChan := make(chan error, 1)
	mc := newMetricContext("security_groups", "delete", resourceGroupName, az.client.SubscriptionID)
	resultChan, proxyErrChan := az.client.Delete(resourceGroupName, networkSecurityGroupName, cancel)
	err := <-proxyErrChan
	mc.Observe(err)
	errChan <- err
	return resultChan, errChan
}

func (az *azSecurityGroupsClient) Get(resourceGroupName string, networkSecurityGroupName string, expand string) (result network.SecurityGroup, err error) {
	err = createArmRateLimitErr(false, "NSGGet")
	if !az.rateLimiterReader.TryAccept() {
		return
	}

	glog.V(10).Infof("azSecurityGroupsClient.Get(%q,%q): start", resourceGroupName, networkSecurityGroupName)
	defer func() {
		glog.V(10).Infof("azSecurityGroupsClient.Get(%q,%q): end", resourceGroupName, networkSecurityGroupName)
	}()

	mc := newMetricContext("security_groups", "get", resourceGroupName, az.client.SubscriptionID)
	result, err = az.client.Get(resourceGroupName, networkSecurityGroupName, expand)
	mc.Observe(err)
	return
}

func (az *azSecurityGroupsClient) List(resourceGroupName string) (result network.SecurityGroupListResult, err error) {
	err = createArmRateLimitErr(false, "NSGList")
	if !az.rateLimiterReader.TryAccept() {
		return
	}

	glog.V(10).Infof("azSecurityGroupsClient.List(%q): start", resourceGroupName)
	defer func() {
		glog.V(10).Infof("azSecurityGroupsClient.List(%q): end", resourceGroupName)
	}()

	mc := newMetricContext("security_groups", "list", resourceGroupName, az.client.SubscriptionID)
	result, err = az.client.List(resourceGroupName)
	mc.Observe(err)
	return
}

// azVirtualMachineScaleSetsClient implements VirtualMachineScaleSetsClient.
type azVirtualMachineScaleSetsClient struct {
<<<<<<< HEAD
	client      computepreview.VirtualMachineScaleSetsClient
	rateLimiter flowcontrol.RateLimiter
=======
	client            compute.VirtualMachineScaleSetsClient
	rateLimiterReader flowcontrol.RateLimiter
	rateLimiterWriter flowcontrol.RateLimiter
>>>>>>> 5bf6b0fd70... WIP - create read/writer rate limiter
}

func newAzVirtualMachineScaleSetsClient(config *azClientConfig) *azVirtualMachineScaleSetsClient {
	virtualMachineScaleSetsClient := computepreview.NewVirtualMachineScaleSetsClient(config.subscriptionID)
	virtualMachineScaleSetsClient.BaseURI = config.resourceManagerEndpoint
	virtualMachineScaleSetsClient.Authorizer = autorest.NewBearerAuthorizer(config.servicePrincipalToken)
	virtualMachineScaleSetsClient.PollingDelay = 5 * time.Second
	configureUserAgent(&virtualMachineScaleSetsClient.Client)

	return &azVirtualMachineScaleSetsClient{
		client:            virtualMachineScaleSetsClient,
		rateLimiterReader: config.rateLimiterReader,
		rateLimiterWriter: config.rateLimiterWriter,
	}
}

<<<<<<< HEAD
func (az *azVirtualMachineScaleSetsClient) CreateOrUpdate(ctx context.Context, resourceGroupName string, VMScaleSetName string, parameters computepreview.VirtualMachineScaleSet) (resp *http.Response, err error) {
	az.rateLimiter.Accept()
=======
func (az *azVirtualMachineScaleSetsClient) CreateOrUpdate(resourceGroupName string, VMScaleSetName string, parameters compute.VirtualMachineScaleSet, cancel <-chan struct{}) (<-chan compute.VirtualMachineScaleSet, <-chan error) {
	/* Write rate limiting */
	if !az.rateLimiterWriter.TryAccept() {
		err := createArmRateLimitErr(true, "VMSSCreateOrUpdate")
		errChan := make(chan error, 1)
		resultChan := make(chan compute.VirtualMachineScaleSet, 1)
		errChan <- err
		resultChan <- compute.VirtualMachineScaleSet{}
		return resultChan, errChan
	}

>>>>>>> 5bf6b0fd70... WIP - create read/writer rate limiter
	glog.V(10).Infof("azVirtualMachineScaleSetsClient.CreateOrUpdate(%q,%q): start", resourceGroupName, VMScaleSetName)
	defer func() {
		glog.V(10).Infof("azVirtualMachineScaleSetsClient.CreateOrUpdate(%q,%q): end", resourceGroupName, VMScaleSetName)
	}()

	mc := newMetricContext("vmss", "create_or_update", resourceGroupName, az.client.SubscriptionID)
	future, err := az.client.CreateOrUpdate(ctx, resourceGroupName, VMScaleSetName, parameters)
	mc.Observe(err)
	if err != nil {
		return future.Response(), err
	}

	err = future.WaitForCompletion(ctx, az.client.Client)
	mc.Observe(err)
	return future.Response(), err
}

<<<<<<< HEAD
func (az *azVirtualMachineScaleSetsClient) Get(ctx context.Context, resourceGroupName string, VMScaleSetName string) (result computepreview.VirtualMachineScaleSet, err error) {
	az.rateLimiter.Accept()
=======
func (az *azVirtualMachineScaleSetsClient) Get(resourceGroupName string, VMScaleSetName string) (result compute.VirtualMachineScaleSet, err error) {
	err = createArmRateLimitErr(false, "VMSSGet")
	if !az.rateLimiterReader.TryAccept() {
		return
	}

>>>>>>> 5bf6b0fd70... WIP - create read/writer rate limiter
	glog.V(10).Infof("azVirtualMachineScaleSetsClient.Get(%q,%q): start", resourceGroupName, VMScaleSetName)
	defer func() {
		glog.V(10).Infof("azVirtualMachineScaleSetsClient.Get(%q,%q): end", resourceGroupName, VMScaleSetName)
	}()

	mc := newMetricContext("vmss", "get", resourceGroupName, az.client.SubscriptionID)
	result, err = az.client.Get(ctx, resourceGroupName, VMScaleSetName)
	mc.Observe(err)
	return
}

<<<<<<< HEAD
func (az *azVirtualMachineScaleSetsClient) List(ctx context.Context, resourceGroupName string) (result []computepreview.VirtualMachineScaleSet, err error) {
	az.rateLimiter.Accept()
=======
func (az *azVirtualMachineScaleSetsClient) List(resourceGroupName string) (result compute.VirtualMachineScaleSetListResult, err error) {
	err = createArmRateLimitErr(false, "VMSSList")
	if !az.rateLimiterReader.TryAccept() {
		return
	}

>>>>>>> 5bf6b0fd70... WIP - create read/writer rate limiter
	glog.V(10).Infof("azVirtualMachineScaleSetsClient.List(%q,%q): start", resourceGroupName)
	defer func() {
		glog.V(10).Infof("azVirtualMachineScaleSetsClient.List(%q,%q): end", resourceGroupName)
	}()

	mc := newMetricContext("vmss", "list", resourceGroupName, az.client.SubscriptionID)
	iterator, err := az.client.ListComplete(ctx, resourceGroupName)
	mc.Observe(err)
	if err != nil {
		return nil, err
	}

<<<<<<< HEAD
	result = make([]computepreview.VirtualMachineScaleSet, 0)
	for ; iterator.NotDone(); err = iterator.Next() {
		if err != nil {
			return nil, err
		}
=======
func (az *azVirtualMachineScaleSetsClient) ListNextResults(resourceGroupName string, lastResults compute.VirtualMachineScaleSetListResult) (result compute.VirtualMachineScaleSetListResult, err error) {
	err = createArmRateLimitErr(false, "VMSSListNextResults")
	if !az.rateLimiterReader.TryAccept() {
		return
	}

	glog.V(10).Infof("azVirtualMachineScaleSetsClient.ListNextResults(%q): start", lastResults)
	defer func() {
		glog.V(10).Infof("azVirtualMachineScaleSetsClient.ListNextResults(%q): end", lastResults)
	}()
>>>>>>> 5bf6b0fd70... WIP - create read/writer rate limiter

		result = append(result, iterator.Value())
	}

	return result, nil
}

<<<<<<< HEAD
func (az *azVirtualMachineScaleSetsClient) UpdateInstances(ctx context.Context, resourceGroupName string, VMScaleSetName string, VMInstanceIDs computepreview.VirtualMachineScaleSetVMInstanceRequiredIDs) (resp *http.Response, err error) {
	az.rateLimiter.Accept()
=======
func (az *azVirtualMachineScaleSetsClient) UpdateInstances(resourceGroupName string, VMScaleSetName string, VMInstanceIDs compute.VirtualMachineScaleSetVMInstanceRequiredIDs, cancel <-chan struct{}) (<-chan compute.OperationStatusResponse, <-chan error) {
	/* Write rate limiting */
	if !az.rateLimiterWriter.TryAccept() {
		err := createArmRateLimitErr(true, "VMSSUpdateInstances")
		errChan := make(chan error, 1)
		resultChan := make(chan compute.OperationStatusResponse, 1)
		errChan <- err
		resultChan <- compute.OperationStatusResponse{}
		return resultChan, errChan
	}

>>>>>>> 5bf6b0fd70... WIP - create read/writer rate limiter
	glog.V(10).Infof("azVirtualMachineScaleSetsClient.UpdateInstances(%q,%q,%q): start", resourceGroupName, VMScaleSetName, VMInstanceIDs)
	defer func() {
		glog.V(10).Infof("azVirtualMachineScaleSetsClient.UpdateInstances(%q,%q,%q): end", resourceGroupName, VMScaleSetName, VMInstanceIDs)
	}()

	mc := newMetricContext("vmss", "update_instances", resourceGroupName, az.client.SubscriptionID)
	future, err := az.client.UpdateInstances(ctx, resourceGroupName, VMScaleSetName, VMInstanceIDs)
	mc.Observe(err)
	if err != nil {
		return future.Response(), err
	}

	err = future.WaitForCompletion(ctx, az.client.Client)
	mc.Observe(err)
	return future.Response(), err
}

// azVirtualMachineScaleSetVMsClient implements VirtualMachineScaleSetVMsClient.
type azVirtualMachineScaleSetVMsClient struct {
<<<<<<< HEAD
	client      computepreview.VirtualMachineScaleSetVMsClient
	rateLimiter flowcontrol.RateLimiter
=======
	client            compute.VirtualMachineScaleSetVMsClient
	rateLimiterReader flowcontrol.RateLimiter
	rateLimiterWriter flowcontrol.RateLimiter
>>>>>>> 5bf6b0fd70... WIP - create read/writer rate limiter
}

func newAzVirtualMachineScaleSetVMsClient(config *azClientConfig) *azVirtualMachineScaleSetVMsClient {
	virtualMachineScaleSetVMsClient := computepreview.NewVirtualMachineScaleSetVMsClient(config.subscriptionID)
	virtualMachineScaleSetVMsClient.BaseURI = config.resourceManagerEndpoint
	virtualMachineScaleSetVMsClient.Authorizer = autorest.NewBearerAuthorizer(config.servicePrincipalToken)
	virtualMachineScaleSetVMsClient.PollingDelay = 5 * time.Second
	configureUserAgent(&virtualMachineScaleSetVMsClient.Client)

	return &azVirtualMachineScaleSetVMsClient{
		client:            virtualMachineScaleSetVMsClient,
		rateLimiterReader: config.rateLimiterReader,
		rateLimiterWriter: config.rateLimiterWriter,
	}
}

<<<<<<< HEAD
func (az *azVirtualMachineScaleSetVMsClient) Get(ctx context.Context, resourceGroupName string, VMScaleSetName string, instanceID string) (result computepreview.VirtualMachineScaleSetVM, err error) {
	az.rateLimiter.Accept()
=======
func (az *azVirtualMachineScaleSetVMsClient) Get(resourceGroupName string, VMScaleSetName string, instanceID string) (result compute.VirtualMachineScaleSetVM, err error) {
	err = createArmRateLimitErr(false, "VMSSGet")
	if !az.rateLimiterReader.TryAccept() {
		return
	}

>>>>>>> 5bf6b0fd70... WIP - create read/writer rate limiter
	glog.V(10).Infof("azVirtualMachineScaleSetVMsClient.Get(%q,%q,%q): start", resourceGroupName, VMScaleSetName, instanceID)
	defer func() {
		glog.V(10).Infof("azVirtualMachineScaleSetVMsClient.Get(%q,%q,%q): end", resourceGroupName, VMScaleSetName, instanceID)
	}()

	mc := newMetricContext("vmssvm", "get", resourceGroupName, az.client.SubscriptionID)
	result, err = az.client.Get(ctx, resourceGroupName, VMScaleSetName, instanceID)
	mc.Observe(err)
	return
}

<<<<<<< HEAD
func (az *azVirtualMachineScaleSetVMsClient) GetInstanceView(ctx context.Context, resourceGroupName string, VMScaleSetName string, instanceID string) (result computepreview.VirtualMachineScaleSetVMInstanceView, err error) {
	az.rateLimiter.Accept()
=======
func (az *azVirtualMachineScaleSetVMsClient) GetInstanceView(resourceGroupName string, VMScaleSetName string, instanceID string) (result compute.VirtualMachineScaleSetVMInstanceView, err error) {
	err = createArmRateLimitErr(false, "VMSSGetInstanceView")
	if !az.rateLimiterReader.TryAccept() {
		return
	}

>>>>>>> 5bf6b0fd70... WIP - create read/writer rate limiter
	glog.V(10).Infof("azVirtualMachineScaleSetVMsClient.GetInstanceView(%q,%q,%q): start", resourceGroupName, VMScaleSetName, instanceID)
	defer func() {
		glog.V(10).Infof("azVirtualMachineScaleSetVMsClient.GetInstanceView(%q,%q,%q): end", resourceGroupName, VMScaleSetName, instanceID)
	}()

	mc := newMetricContext("vmssvm", "get_instance_view", resourceGroupName, az.client.SubscriptionID)
	result, err = az.client.GetInstanceView(ctx, resourceGroupName, VMScaleSetName, instanceID)
	mc.Observe(err)
	return
}

<<<<<<< HEAD
func (az *azVirtualMachineScaleSetVMsClient) List(ctx context.Context, resourceGroupName string, virtualMachineScaleSetName string, filter string, selectParameter string, expand string) (result []computepreview.VirtualMachineScaleSetVM, err error) {
	az.rateLimiter.Accept()
=======
func (az *azVirtualMachineScaleSetVMsClient) List(resourceGroupName string, virtualMachineScaleSetName string, filter string, selectParameter string, expand string) (result compute.VirtualMachineScaleSetVMListResult, err error) {
	err = createArmRateLimitErr(false, "VMSSList")
	if !az.rateLimiterReader.TryAccept() {
		return
	}

>>>>>>> 5bf6b0fd70... WIP - create read/writer rate limiter
	glog.V(10).Infof("azVirtualMachineScaleSetVMsClient.List(%q,%q,%q): start", resourceGroupName, virtualMachineScaleSetName, filter)
	defer func() {
		glog.V(10).Infof("azVirtualMachineScaleSetVMsClient.List(%q,%q,%q): end", resourceGroupName, virtualMachineScaleSetName, filter)
	}()

	mc := newMetricContext("vmssvm", "list", resourceGroupName, az.client.SubscriptionID)
	iterator, err := az.client.ListComplete(ctx, resourceGroupName, virtualMachineScaleSetName, filter, selectParameter, expand)
	mc.Observe(err)
	if err != nil {
		return nil, err
	}

	result = make([]computepreview.VirtualMachineScaleSetVM, 0)
	for ; iterator.NotDone(); err = iterator.Next() {
		if err != nil {
			return nil, err
		}

		result = append(result, iterator.Value())
	}

	return result, nil
}

<<<<<<< HEAD
func (az *azVirtualMachineScaleSetVMsClient) Update(ctx context.Context, resourceGroupName string, VMScaleSetName string, instanceID string, parameters computepreview.VirtualMachineScaleSetVM) (resp *http.Response, err error) {
	az.rateLimiter.Accept()
	glog.V(10).Infof("azVirtualMachineScaleSetVMsClient.Update(%q,%q,%q): start", resourceGroupName, VMScaleSetName, instanceID)
=======
func (az *azVirtualMachineScaleSetVMsClient) ListNextResults(resourceGroupName string, lastResults compute.VirtualMachineScaleSetVMListResult) (result compute.VirtualMachineScaleSetVMListResult, err error) {
	err = createArmRateLimitErr(false, "VMSSListNextResults")
	if !az.rateLimiterReader.TryAccept() {
		return
	}

	glog.V(10).Infof("azVirtualMachineScaleSetVMsClient.ListNextResults(%q,%q,%q): start", lastResults)
>>>>>>> 5bf6b0fd70... WIP - create read/writer rate limiter
	defer func() {
		glog.V(10).Infof("azVirtualMachineScaleSetVMsClient.Update(%q,%q,%q): end", resourceGroupName, VMScaleSetName, instanceID)
	}()

	mc := newMetricContext("vmssvm", "update", resourceGroupName, az.client.SubscriptionID)
	future, err := az.client.Update(ctx, resourceGroupName, VMScaleSetName, instanceID, parameters)
	mc.Observe(err)
	if err != nil {
		return future.Response(), err
	}

	err = future.WaitForCompletion(ctx, az.client.Client)
	mc.Observe(err)
	return future.Response(), err
}

// azRoutesClient implements RoutesClient.
type azRoutesClient struct {
	client            network.RoutesClient
	rateLimiterReader flowcontrol.RateLimiter
	rateLimiterWriter flowcontrol.RateLimiter
}

func newAzRoutesClient(config *azClientConfig) *azRoutesClient {
	routesClient := network.NewRoutesClient(config.subscriptionID)
	routesClient.BaseURI = config.resourceManagerEndpoint
	routesClient.Authorizer = autorest.NewBearerAuthorizer(config.servicePrincipalToken)
	routesClient.PollingDelay = 5 * time.Second
	configureUserAgent(&routesClient.Client)

	return &azRoutesClient{
		client:            routesClient,
		rateLimiterReader: config.rateLimiterReader,
		rateLimiterWriter: config.rateLimiterWriter,
	}
}

func (az *azRoutesClient) CreateOrUpdate(resourceGroupName string, routeTableName string, routeName string, routeParameters network.Route, cancel <-chan struct{}) (<-chan network.Route, <-chan error) {
	/* Write rate limiting */
	if !az.rateLimiterWriter.TryAccept() {
		err := createArmRateLimitErr(true, "RouteCreateOrUpdate")
		errChan := make(chan error, 1)
		resultChan := make(chan network.Route, 1)
		errChan <- err
		resultChan <- network.Route{}
		return resultChan, errChan
	}

	glog.V(10).Infof("azRoutesClient.CreateOrUpdate(%q,%q,%q): start", resourceGroupName, routeTableName, routeName)
	defer func() {
		glog.V(10).Infof("azRoutesClient.CreateOrUpdate(%q,%q,%q): end", resourceGroupName, routeTableName, routeName)
	}()

	errChan := make(chan error, 1)
	mc := newMetricContext("routes", "create_or_update", resourceGroupName, az.client.SubscriptionID)
	resultChan, proxyErrChan := az.client.CreateOrUpdate(resourceGroupName, routeTableName, routeName, routeParameters, cancel)
	err := <-proxyErrChan
	mc.Observe(err)
	errChan <- err
	return resultChan, errChan
}

func (az *azRoutesClient) Delete(resourceGroupName string, routeTableName string, routeName string, cancel <-chan struct{}) (<-chan autorest.Response, <-chan error) {
	/* Write rate limiting */
	if !az.rateLimiterWriter.TryAccept() {
		err := createArmRateLimitErr(true, "RouteDelete")
		errChan := make(chan error, 1)
		resultChan := make(chan autorest.Response, 1)
		errChan <- err
		resultChan <- autorest.Response{}
		return resultChan, errChan
	}

	glog.V(10).Infof("azRoutesClient.Delete(%q,%q,%q): start", resourceGroupName, routeTableName, routeName)
	defer func() {
		glog.V(10).Infof("azRoutesClient.Delete(%q,%q,%q): end", resourceGroupName, routeTableName, routeName)
	}()

	errChan := make(chan error, 1)
	mc := newMetricContext("routes", "delete", resourceGroupName, az.client.SubscriptionID)
	resultChan, proxyErrChan := az.client.Delete(resourceGroupName, routeTableName, routeName, cancel)
	err := <-proxyErrChan
	mc.Observe(err)
	errChan <- err
	return resultChan, errChan
}

// azRouteTablesClient implements RouteTablesClient.
type azRouteTablesClient struct {
	client            network.RouteTablesClient
	rateLimiterReader flowcontrol.RateLimiter
	rateLimiterWriter flowcontrol.RateLimiter
}

func newAzRouteTablesClient(config *azClientConfig) *azRouteTablesClient {
	routeTablesClient := network.NewRouteTablesClient(config.subscriptionID)
	routeTablesClient.BaseURI = config.resourceManagerEndpoint
	routeTablesClient.Authorizer = autorest.NewBearerAuthorizer(config.servicePrincipalToken)
	routeTablesClient.PollingDelay = 5 * time.Second
	configureUserAgent(&routeTablesClient.Client)

	return &azRouteTablesClient{
		client:            routeTablesClient,
		rateLimiterReader: config.rateLimiterReader,
		rateLimiterWriter: config.rateLimiterWriter,
	}
}

func (az *azRouteTablesClient) CreateOrUpdate(resourceGroupName string, routeTableName string, parameters network.RouteTable, cancel <-chan struct{}) (<-chan network.RouteTable, <-chan error) {
	/* Write rate limiting */
	if !az.rateLimiterWriter.TryAccept() {
		err := createArmRateLimitErr(true, "RouteTableCreateOrUpdate")
		errChan := make(chan error, 1)
		resultChan := make(chan network.RouteTable, 1)
		errChan <- err
		resultChan <- network.RouteTable{}
		return resultChan, errChan
	}

	glog.V(10).Infof("azRouteTablesClient.CreateOrUpdate(%q,%q): start", resourceGroupName, routeTableName)
	defer func() {
		glog.V(10).Infof("azRouteTablesClient.CreateOrUpdate(%q,%q): end", resourceGroupName, routeTableName)
	}()

	errChan := make(chan error, 1)
	mc := newMetricContext("route_tables", "create_or_update", resourceGroupName, az.client.SubscriptionID)
	resultChan, proxyErrChan := az.client.CreateOrUpdate(resourceGroupName, routeTableName, parameters, cancel)
	err := <-proxyErrChan
	mc.Observe(err)
	errChan <- err
	return resultChan, errChan
}

func (az *azRouteTablesClient) Get(resourceGroupName string, routeTableName string, expand string) (result network.RouteTable, err error) {
	err = createArmRateLimitErr(false, "GetRouteTable")
	if !az.rateLimiterReader.TryAccept() {
		return
	}

	glog.V(10).Infof("azRouteTablesClient.Get(%q,%q): start", resourceGroupName, routeTableName)
	defer func() {
		glog.V(10).Infof("azRouteTablesClient.Get(%q,%q): end", resourceGroupName, routeTableName)
	}()

	mc := newMetricContext("route_tables", "get", resourceGroupName, az.client.SubscriptionID)
	result, err = az.client.Get(resourceGroupName, routeTableName, expand)
	mc.Observe(err)
	return
}

// azStorageAccountClient implements StorageAccountClient.
type azStorageAccountClient struct {
	client            storage.AccountsClient
	rateLimiterReader flowcontrol.RateLimiter
	rateLimiterWriter flowcontrol.RateLimiter
}

func newAzStorageAccountClient(config *azClientConfig) *azStorageAccountClient {
	storageAccountClient := storage.NewAccountsClientWithBaseURI(config.resourceManagerEndpoint, config.subscriptionID)
	storageAccountClient.Authorizer = autorest.NewBearerAuthorizer(config.servicePrincipalToken)
	storageAccountClient.PollingDelay = 5 * time.Second
	configureUserAgent(&storageAccountClient.Client)

	return &azStorageAccountClient{
		client:            storageAccountClient,
		rateLimiterReader: config.rateLimiterReader,
		rateLimiterWriter: config.rateLimiterWriter,
	}
}

func (az *azStorageAccountClient) Create(resourceGroupName string, accountName string, parameters storage.AccountCreateParameters, cancel <-chan struct{}) (<-chan storage.Account, <-chan error) {
	/* Write rate limiting */
	if !az.rateLimiterWriter.TryAccept() {
		err := createArmRateLimitErr(true, "StorageAccountCreate")
		errChan := make(chan error, 1)
		resultChan := make(chan storage.Account, 1)
		errChan <- err
		resultChan <- storage.Account{}
		return resultChan, errChan
	}

	glog.V(10).Infof("azStorageAccountClient.Create(%q,%q): start", resourceGroupName, accountName)
	defer func() {
		glog.V(10).Infof("azStorageAccountClient.Create(%q,%q): end", resourceGroupName, accountName)
	}()

	errChan := make(chan error, 1)
	mc := newMetricContext("storage_account", "create", resourceGroupName, az.client.SubscriptionID)
	resultChan, proxyErrChan := az.client.Create(resourceGroupName, accountName, parameters, cancel)
	err := <-proxyErrChan
	mc.Observe(err)
	errChan <- err
	return resultChan, errChan
}

func (az *azStorageAccountClient) Delete(resourceGroupName string, accountName string) (result autorest.Response, err error) {
	err = createArmRateLimitErr(false, "DeleteStorageAccount")
	if !az.rateLimiterReader.TryAccept() {
		return
	}

	glog.V(10).Infof("azStorageAccountClient.Delete(%q,%q): start", resourceGroupName, accountName)
	defer func() {
		glog.V(10).Infof("azStorageAccountClient.Delete(%q,%q): end", resourceGroupName, accountName)
	}()

	mc := newMetricContext("storage_account", "delete", resourceGroupName, az.client.SubscriptionID)
	result, err = az.client.Delete(resourceGroupName, accountName)
	mc.Observe(err)
	return
}

func (az *azStorageAccountClient) ListKeys(resourceGroupName string, accountName string) (result storage.AccountListKeysResult, err error) {
	err = createArmRateLimitErr(false, "ListStorageAccountKeys")
	if !az.rateLimiterReader.TryAccept() {
		return
	}

	glog.V(10).Infof("azStorageAccountClient.ListKeys(%q,%q): start", resourceGroupName, accountName)
	defer func() {
		glog.V(10).Infof("azStorageAccountClient.ListKeys(%q,%q): end", resourceGroupName, accountName)
	}()

	mc := newMetricContext("storage_account", "list_keys", resourceGroupName, az.client.SubscriptionID)
	result, err = az.client.ListKeys(resourceGroupName, accountName)
	mc.Observe(err)
	return
}

func (az *azStorageAccountClient) ListByResourceGroup(resourceGroupName string) (result storage.AccountListResult, err error) {
	err = createArmRateLimitErr(false, "ListStorageAccountsByResourceGroup")
	if !az.rateLimiterReader.TryAccept() {
		return
	}

	glog.V(10).Infof("azStorageAccountClient.ListByResourceGroup(%q): start", resourceGroupName)
	defer func() {
		glog.V(10).Infof("azStorageAccountClient.ListByResourceGroup(%q): end", resourceGroupName)
	}()

	mc := newMetricContext("storage_account", "list_by_resource_group", resourceGroupName, az.client.SubscriptionID)
	result, err = az.client.ListByResourceGroup(resourceGroupName)
	mc.Observe(err)
	return
}

func (az *azStorageAccountClient) GetProperties(resourceGroupName string, accountName string) (result storage.Account, err error) {
	err = createArmRateLimitErr(false, "GetStorageAccount/Properties")
	if !az.rateLimiterReader.TryAccept() {
		return
	}

	glog.V(10).Infof("azStorageAccountClient.GetProperties(%q,%q): start", resourceGroupName, accountName)
	defer func() {
		glog.V(10).Infof("azStorageAccountClient.GetProperties(%q,%q): end", resourceGroupName, accountName)
	}()

	mc := newMetricContext("storage_account", "get_properties", resourceGroupName, az.client.SubscriptionID)
	result, err = az.client.GetProperties(resourceGroupName, accountName)
	mc.Observe(err)
	return
}

// azDisksClient implements DisksClient.
type azDisksClient struct {
	client            disk.DisksClient
	rateLimiterReader flowcontrol.RateLimiter
	rateLimiterWriter flowcontrol.RateLimiter
}

func newAzDisksClient(config *azClientConfig) *azDisksClient {
	disksClient := disk.NewDisksClientWithBaseURI(config.resourceManagerEndpoint, config.subscriptionID)
	disksClient.Authorizer = autorest.NewBearerAuthorizer(config.servicePrincipalToken)
	disksClient.PollingDelay = 5 * time.Second
	configureUserAgent(&disksClient.Client)

	return &azDisksClient{
		client:            disksClient,
		rateLimiterReader: config.rateLimiterReader,
		rateLimiterWriter: config.rateLimiterWriter,
	}
}

func (az *azDisksClient) CreateOrUpdate(resourceGroupName string, diskName string, diskParameter disk.Model, cancel <-chan struct{}) (<-chan disk.Model, <-chan error) {

	/* Write rate limiting */
	if !az.rateLimiterWriter.TryAccept() {
		err := createArmRateLimitErr(true, "DiskCreateOrUpdate")
		errChan := make(chan error, 1)
		resultChan := make(chan disk.Model, 1)
		errChan <- err
		resultChan <- disk.Model{}
		return resultChan, errChan
	}

	glog.V(10).Infof("azDisksClient.CreateOrUpdate(%q,%q): start", resourceGroupName, diskName)
	defer func() {
		glog.V(10).Infof("azDisksClient.CreateOrUpdate(%q,%q): end", resourceGroupName, diskName)
	}()

	errChan := make(chan error, 1)
	mc := newMetricContext("disks", "create_or_update", resourceGroupName, az.client.SubscriptionID)
	resultChan, proxyErrChan := az.client.CreateOrUpdate(resourceGroupName, diskName, diskParameter, cancel)
	err := <-proxyErrChan
	mc.Observe(err)
	errChan <- err
	return resultChan, errChan
}

func (az *azDisksClient) Delete(resourceGroupName string, diskName string, cancel <-chan struct{}) (<-chan disk.OperationStatusResponse, <-chan error) {
	/* Write rate limiting */
	if !az.rateLimiterWriter.TryAccept() {
		err := createArmRateLimitErr(true, "DiskDelete")
		errChan := make(chan error, 1)
		resultChan := make(chan disk.OperationStatusResponse, 1)
		errChan <- err
		resultChan <- disk.OperationStatusResponse{}
		return resultChan, errChan
	}

	glog.V(10).Infof("azDisksClient.Delete(%q,%q): start", resourceGroupName, diskName)
	defer func() {
		glog.V(10).Infof("azDisksClient.Delete(%q,%q): end", resourceGroupName, diskName)
	}()

	errChan := make(chan error, 1)
	mc := newMetricContext("disks", "delete", resourceGroupName, az.client.SubscriptionID)
	resultChan, proxyErrChan := az.client.Delete(resourceGroupName, diskName, cancel)
	err := <-proxyErrChan
	mc.Observe(err)
	errChan <- err
	return resultChan, errChan
}

func (az *azDisksClient) Get(resourceGroupName string, diskName string) (result disk.Model, err error) {
	err = createArmRateLimitErr(false, "GetDisk")
	if !az.rateLimiterReader.TryAccept() {
		return
	}

	glog.V(10).Infof("azDisksClient.Get(%q,%q): start", resourceGroupName, diskName)
	defer func() {
		glog.V(10).Infof("azDisksClient.Get(%q,%q): end", resourceGroupName, diskName)
	}()

	mc := newMetricContext("disks", "get", resourceGroupName, az.client.SubscriptionID)
	result, err = az.client.Get(resourceGroupName, diskName)
	mc.Observe(err)
	return
}
