// Copyright 2019 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rpc

import (
	"context"
	"fmt"
	"strings"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/protobuf/ptypes/timestamp"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto"

	"go.chromium.org/luci/gce/api/instances/v1"
	"go.chromium.org/luci/gce/appengine/model"
	"go.chromium.org/luci/gce/vmtoken"
)

// Instances implements instances.InstancesServer.
type Instances struct {
}

// Ensure Instances implements instances.InstancesServer.
var _ instances.InstancesServer = &Instances{}

// Delete handles a request to delete an instance asynchronously.
func (*Instances) Delete(c context.Context, req *instances.DeleteRequest) (*empty.Empty, error) {
	if req.GetId() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "ID is required")
	}
	vm := &model.VM{
		ID: req.Id,
	}
	if err := datastore.RunInTransaction(c, func(c context.Context) error {
		switch err := datastore.Get(c, vm); {
		case err == datastore.ErrNoSuchEntity:
			return nil
		case err != nil:
			return errors.Annotate(err, "failed to fetch VM").Err()
		case vm.Drained:
			return nil
		}
		vm.Drained = true
		if err := datastore.Put(c, vm); err != nil {
			return errors.Annotate(err, "failed to store VM").Err()
		}
		return nil
	}, nil); err != nil {
		return nil, err
	}
	return &empty.Empty{}, nil
}

// toInstance returns an *instances.Instance representation of the given
// *model.VM.
func toInstance(vm *model.VM) *instances.Instance {
	inst := &instances.Instance{
		Id:                vm.ID,
		ConfigRevision:    vm.Revision,
		Disks:             make([]*instances.Disk, len(vm.Attributes.Disk)),
		Drained:           vm.Drained,
		Hostname:          vm.Hostname,
		NetworkInterfaces: make([]*instances.NetworkInterface, len(vm.NetworkInterfaces)),
		Lifetime:          vm.Lifetime,
		Prefix:            vm.Prefix,
		Project:           vm.Attributes.Project,
		Swarming:          vm.Swarming,
		Timeout:           vm.Timeout,
		Zone:              vm.Attributes.Zone,
	}
	for i, d := range vm.Attributes.Disk {
		inst.Disks[i] = &instances.Disk{
			Image: d.Image,
		}
	}
	for i, n := range vm.NetworkInterfaces {
		inst.NetworkInterfaces[i] = &instances.NetworkInterface{
			InternalIp: n.InternalIP,
		}
		// GCE currently supports at most one external IP address per network interface.
		if n.ExternalIP != "" {
			inst.NetworkInterfaces[i].ExternalIps = []string{n.ExternalIP}
		}
	}
	if vm.Created > 0 {
		inst.Created = &timestamp.Timestamp{
			Seconds: vm.Created,
		}
	}
	if vm.Connected > 0 {
		inst.Connected = &timestamp.Timestamp{
			Seconds: vm.Connected,
		}
	}
	return inst
}

// getByID returns the *instances.Instance matching the given ID. Always returns
// permission denied when called by VMs.
func getByID(c context.Context, id string) (*instances.Instance, error) {
	if vmtoken.Has(c) {
		return nil, status.Errorf(codes.PermissionDenied, "unauthorized user")
	}
	vm := &model.VM{
		ID: id,
	}
	switch err := datastore.Get(c, vm); {
	case err == datastore.ErrNoSuchEntity:
		return nil, status.Errorf(codes.NotFound, "no VM found with ID %q", id)
	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch VM").Err()
	default:
		return toInstance(vm), nil
	}
}

// getByHostname returns the *instances.Instance matching the given hostname.
// May be called by VMs.
func getByHostname(c context.Context, hostname string) (*instances.Instance, error) {
	if vmtoken.Has(c) && vmtoken.Hostname(c) != hostname {
		return nil, status.Errorf(codes.PermissionDenied, "unauthorized user")
	}
	var vms []*model.VM
	// Hostnames are globally unique, so there should be at most one match.
	q := datastore.NewQuery(model.VMKind).Eq("hostname", hostname).Limit(1)
	switch err := datastore.GetAll(c, q, &vms); {
	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch VM").Err()
	case len(vms) == 0:
		if vmtoken.Has(c) {
			return nil, status.Errorf(codes.PermissionDenied, "unauthorized user")
		}
		return nil, status.Errorf(codes.NotFound, "no VM found with hostname %q", hostname)
	default:
		inst := toInstance(vms[0])
		if vmtoken.Has(c) {
			if !vmtoken.Matches(c, inst.Hostname, inst.Zone, inst.Project) {
				return nil, status.Errorf(codes.PermissionDenied, "unauthorized user")
			}
			// Allow VMs to view minimal self-information.
			inst = &instances.Instance{
				Hostname: inst.Hostname,
				Swarming: inst.Swarming,
			}
		}
		return inst, nil
	}
}

// Get handles a request to get an existing instance.
func (*Instances) Get(c context.Context, req *instances.GetRequest) (*instances.Instance, error) {
	switch {
	case req.GetId() == "" && req.GetHostname() == "":
		return nil, status.Errorf(codes.InvalidArgument, "ID or hostname is required")
	case req.Id != "" && req.Hostname != "":
		return nil, status.Errorf(codes.InvalidArgument, "exactly one of ID or hostname is required")
	case req.Id != "":
		return getByID(c, req.Id)
	default:
		return getByHostname(c, req.Hostname)
	}
}

// List handles a request to list instances.
func (*Instances) List(c context.Context, req *instances.ListRequest) (*instances.ListResponse, error) {
	q := datastore.NewQuery(model.VMKind)
	if req.GetPrefix() != "" {
		q = q.Eq("prefix", req.Prefix)
	}
	if req.GetFilter() != "" {
		// TODO(crbug/964591): Support other filters.
		// No compound indices exist, so only simple filters are supported:
		// https://cloud.google.com/datastore/docs/concepts/indexes#index_configuration.
		if !strings.HasPrefix(req.Filter, "disks.image=") {
			return nil, status.Errorf(codes.InvalidArgument, "invalid filter expression %q", req.Filter)
		}
		img := strings.TrimPrefix(req.Filter, "disks.image=")
		q = q.Eq("attributes_indexed", fmt.Sprintf("disk.image:%s", img))
	}

	rsp := &instances.ListResponse{}
	if err := proto.PageQuery(c, req, rsp, q, func(vm *model.VM) error {
		rsp.Instances = append(rsp.Instances, toInstance(vm))
		return nil
	}); err != nil {
		return nil, err
	}
	return rsp, nil
}

// NewInstancesServer returns a new instances server.
func NewInstancesServer() instances.InstancesServer {
	return &instances.DecoratedInstances{
		Prelude:  vmAccessPrelude,
		Service:  &Instances{},
		Postlude: gRPCifyAndLogErr,
	}
}
