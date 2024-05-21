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

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/paged"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gce/api/instances/v1"
	"go.chromium.org/luci/gce/appengine/model"
	"go.chromium.org/luci/gce/appengine/rpc/internal/metrics"
	"go.chromium.org/luci/gce/vmtoken"
	"go.chromium.org/luci/server/auth"
)

// Instances implements instances.InstancesServer.
type Instances struct {
}

// Ensure Instances implements instances.InstancesServer.
var _ instances.InstancesServer = &Instances{}

// deleteByID asynchronously deletes the instance matching the given ID.
func deleteByID(c context.Context, id string) (*emptypb.Empty, error) {
	vm := &model.VM{
		ID: id,
	}
	if err := datastore.RunInTransaction(c, func(c context.Context) error {
		switch err := datastore.Get(c, vm); {
		case errors.Is(err, datastore.ErrNoSuchEntity):
			return nil
		case err != nil:
			return errors.Annotate(err, "failed to fetch VM with id: %q", id).Err()
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
	return &emptypb.Empty{}, nil
}

// deleteByHostname asynchronously deletes the instance matching the given
// hostname.
func deleteByHostname(c context.Context, hostname string) (*emptypb.Empty, error) {
	var vms []*model.VM
	// Hostnames are globally unique, so there should be at most one match.
	q := datastore.NewQuery(model.VMKind).Eq("hostname", hostname).Limit(1)
	switch err := datastore.GetAll(c, q, &vms); {
	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch VM with hostname: %q", hostname).Err()
	case len(vms) == 0:
		return &emptypb.Empty{}, nil
	default:
		return deleteByID(c, vms[0].ID)
	}
}

// Delete handles a request to delete an instance asynchronously.
func (*Instances) Delete(c context.Context, req *instances.DeleteRequest) (*emptypb.Empty, error) {
	switch {
	case req.GetId() == "" && req.GetHostname() == "":
		return nil, status.Errorf(codes.InvalidArgument, "ID or hostname is required")
	case req.Id != "" && req.Hostname != "":
		return nil, status.Errorf(codes.InvalidArgument, "exactly one of ID or hostname is required")
	case req.Id != "":
		return deleteByID(c, req.Id)
	default:
		return deleteByHostname(c, req.Hostname)
	}
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
		inst.Created = &timestamppb.Timestamp{
			Seconds: vm.Created,
		}
	}
	if vm.Connected > 0 {
		inst.Connected = &timestamppb.Timestamp{
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
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil, status.Errorf(codes.NotFound, "no VM found with ID %q", id)
	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch VM with id: %q", id).Err()
	default:
		return toInstance(vm), nil
	}
}

// getByHostname returns the *instances.Instance matching the given hostname.
// May be called by VMs.
func getByHostname(c context.Context, hostname string) (*instances.Instance, error) {
	if vmtoken.Has(c) && vmtoken.Hostname(c) != hostname {
		logging.Warningf(c, "VM %q trying to get host %q", vmtoken.Hostname(c), hostname)
		return nil, status.Errorf(codes.PermissionDenied, "unauthorized user")
	}
	var vms []*model.VM
	// Hostnames are globally unique, so there should be at most one match.
	q := datastore.NewQuery(model.VMKind).Eq("hostname", hostname).Limit(1)
	switch err := datastore.GetAll(c, q, &vms); {
	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch VM with hostname: %q", hostname).Err()
	case len(vms) == 0:
		if vmtoken.Has(c) {
			metrics.UpdateUntrackedGets(c, hostname)
			logging.Warningf(c, "no VMs found with hostname %q", hostname)
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
		if !strings.HasPrefix(req.Filter, "instances.disks.image=") {
			return nil, status.Errorf(codes.InvalidArgument, "invalid filter expression %q", req.Filter)
		}
		img := strings.TrimPrefix(req.Filter, "instances.disks.image=")
		q = q.Eq("attributes_indexed", fmt.Sprintf("disk.image:%s", img))
	}
	lim := req.GetPageSize()
	if lim < 1 || lim > 5000 {
		lim = 5000
	}

	rsp := &instances.ListResponse{}
	if err := paged.Query(c, lim, req.GetPageToken(), rsp, q, func(vm *model.VM) error {
		rsp.Instances = append(rsp.Instances, toInstance(vm))
		return nil
	}); err != nil {
		return nil, err
	}
	return rsp, nil
}

// instancesPrelude ensures the user is authorized to use the instances API. VMs
// have limited access to Get.
func instancesPrelude(c context.Context, methodName string, req proto.Message) (context.Context, error) {
	groups := []string{admins, writers}
	if isReadOnly(methodName) {
		groups = append(groups, readers)
	}
	switch is, err := auth.IsMember(c, groups...); {
	case err != nil:
		return c, err
	case is:
		logging.Debugf(c, "%s called %q:\n%s", auth.CurrentIdentity(c), methodName, req)
		// Remove the VM token if the caller also has OAuth2-based access.
		// This is because VMs have additional restrictions when using the API.
		return vmtoken.Clear(c), nil
	case methodName == "Get" && vmtoken.Has(c):
		// Get applies additional restrictions when the caller is a VM.
		logging.Debugf(c, "%s called %q:\n%s", vmtoken.CurrentIdentity(c), methodName, req)
		return c, nil
	}
	return c, status.Errorf(codes.PermissionDenied, "unauthorized user")
}

// NewInstancesServer returns a new instances server.
func NewInstancesServer() instances.InstancesServer {
	return &instances.DecoratedInstances{
		Prelude:  instancesPrelude,
		Service:  &Instances{},
		Postlude: gRPCifyAndLogErr,
	}
}
