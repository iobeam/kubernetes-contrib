/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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

package main

import (
	compute "google.golang.org/api/compute/v1"
)

// This is the structure of the gce l7 controller:
// apiserver <-> controller ---> pools --> cloud
//                  |               |
//                  |-> Ingress     |-> backends
//                  |-> Services    |   |-> health checks
//                  |-> Nodes       |
//                                  |-> instance groups
//                                  |   |-> port per backend
//                                  |
//                                  |-> loadbalancers
//                                      |-> http proxy
//                                      |-> forwarding rule
//                                      |-> urlmap
// * apiserver: kubernetes api serer.
// * controller: gce l7 controller, watches apiserver and interacts
//	with sync pools. The controller doesn't know anything about the cloud.
//  Communication between the controller and pools is 1 way.
// * pool: the controller tells each pool about desired state by inserting
//	into shared memory store. The pools sync this with the cloud. Pools are
//  also responsible for periodically checking the edge links between various
//	cloud resources.
//
// To adequately test this, we need 1 set of interfaces from pools -> cloud,
// and another from controller -> pools. All the cloud interfaces have a
// corresponding implementation in fakes.go. When the controller runs in
// production (i.e --running-in-cluster=true) it passes a Kubernetes
// cloudprovider object to each of the sync pools. During unittests or when run
// with --running-in-cluster=false, each sync pool gets a fake cloud interface.

// A note on naming convention: per golang style guide for Initialisms, Http
// should be HTTP and Url should be URL, however because these interfaces
// must match their siblings in the Kubernetes cloud provider, which are in turn
// consistent with GCE compute API, there might be inconsistencies.

// NodePool is an interface to manage a pool of kubernetes nodes synced with vm instances in the cloud
// through the InstanceGroups interface.
type NodePool interface {
	Add(nodeNames []string) error
	Remove(nodeNames []string) error
	Sync(nodeNames []string) error
	// TODO: Leaks the cloud abstraction.
	Get(name string) (*compute.InstanceGroup, error)
	Shutdown() error
}

// InstanceGroups is an interface for managing gce instances groups, and the instances therein.
type InstanceGroups interface {
	GetInstanceGroup(name string) (*compute.InstanceGroup, error)
	CreateInstanceGroup(name string) (*compute.InstanceGroup, error)
	DeleteInstanceGroup(name string) error

	// TODO: Refactor for modulatiry.
	ListInstancesInInstanceGroup(name string, state string) (*compute.InstanceGroupsListInstances, error)
	AddInstancesToInstanceGroup(name string, instanceNames []string) error
	RemoveInstancesFromInstanceGroup(name string, instanceName []string) error
	AddPortToInstanceGroup(ig *compute.InstanceGroup, port int64) (*compute.NamedPort, error)
}

// BackendPool is an interface to manage a pool of kubernetes nodePort services
// as gce backendServices, and sync them through the BackendServices interface.
type BackendPool interface {
	Add(port int64) error
	Get(port int64) (*compute.BackendService, error)
	Delete(port int64) error
	Sync(ports []int64) error
	GC(ports []int64) error
	Shutdown() error
}

// BackendServices is an interface for managing gce backend services.
type BackendServices interface {
	GetBackendService(name string) (*compute.BackendService, error)
	UpdateBackendService(bg *compute.BackendService) error
	CreateBackendService(bg *compute.BackendService) error
	DeleteBackendService(name string) error
}

// LoadBalancers is an interface for managing all the gce resources needed by L7
// loadbalancers. We don't have individual pools for each of these resources
// because none of them are usable (or acquirable) stand-alone, unlinke backends
// and instance groups. The dependency graph:
// ForwardingRule -> UrlMaps -> TargetProxies
type LoadBalancers interface {
	// Forwarding Rules
	GetGlobalForwardingRule(name string) (*compute.ForwardingRule, error)
	CreateGlobalForwardingRule(proxy *compute.TargetHttpProxy, name string, portRange string) (*compute.ForwardingRule, error)
	DeleteGlobalForwardingRule(name string) error
	SetProxyForGlobalForwardingRule(fw *compute.ForwardingRule, proxy *compute.TargetHttpProxy) error

	// UrlMaps
	GetUrlMap(name string) (*compute.UrlMap, error)
	CreateUrlMap(backend *compute.BackendService, name string) (*compute.UrlMap, error)
	UpdateUrlMap(urlMap *compute.UrlMap) (*compute.UrlMap, error)
	DeleteUrlMap(name string) error

	// TargetProxies
	GetTargetHttpProxy(name string) (*compute.TargetHttpProxy, error)
	CreateTargetHttpProxy(urlMap *compute.UrlMap, name string) (*compute.TargetHttpProxy, error)
	DeleteTargetHttpProxy(name string) error
	SetUrlMapForTargetHttpProxy(proxy *compute.TargetHttpProxy, urlMap *compute.UrlMap) error
}

// LoadBalancerPool is an interface to manage the cloud resources associated
// with a gce loadbalancer.
type LoadBalancerPool interface {
	Get(name string) (*L7, error)
	Add(name string) error
	Delete(name string) error
	Sync(names []string) error
	GC(names []string) error
	Shutdown() error
}

// SingleHealthCheck is an interface to manage a single GCE health check.
type SingleHealthCheck interface {
	CreateHttpHealthCheck(hc *compute.HttpHealthCheck) error
	DeleteHttpHealthCheck(name string) error
	GetHttpHealthCheck(name string) (*compute.HttpHealthCheck, error)
}

// HealthChecker is an interface to manage the cloud resources associated with
// health checking. Currently it's just a think wrapper around HealthCheck.
type HealthChecker interface {
	Add(name string) error
	Delete(name string) error
	Get(name string) (*compute.HttpHealthCheck, error)
}
