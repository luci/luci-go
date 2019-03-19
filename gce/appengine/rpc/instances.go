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

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/protobuf/ptypes/timestamp"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/gce/api/instances/v1"
	"go.chromium.org/luci/gce/appengine/model"
)

// Instances implements instances.InstancesServer.
type Instances struct {
}

// Ensure Instances implements instances.InstancesServer.
var _ instances.InstancesServer = &Instances{}

// Delete handles a request to delete an instance.
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

// vmToInstance returns an *instances.Instance for the given *model.VM.
func vmToInstance(vm *model.VM) *instances.Instance {
	inst := &instances.Instance{
		Id:       vm.ID,
		Drained:  vm.Drained,
		Hostname: vm.Hostname,
		Lifetime: vm.Lifetime,
		Swarming: vm.Swarming,
		Timeout:  vm.Timeout,
	}
	if vm.Connected > 0 {
		inst.Connected = &timestamp.Timestamp{
			Seconds: vm.Connected,
		}
	}
	if vm.Created > 0 {
		inst.Created = &timestamp.Timestamp{
			Seconds: vm.Created,
		}
	}
	return inst
}

// Get handles a request to get an instance.
func (*Instances) Get(c context.Context, req *instances.GetRequest) (*instances.Instance, error) {
	if req.GetId() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "ID is required")
	}
	vm := &model.VM{
		ID: req.Id,
	}
	switch err := datastore.Get(c, vm); {
	case err == datastore.ErrNoSuchEntity:
		return nil, status.Errorf(codes.NotFound, "no VM found with ID %q", req.Id)
	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch VM").Err()
	}
	return vmToInstance(vm), nil
}

// List handles a request to list instances.
func (*Instances) List(c context.Context, req *instances.ListRequest) (*instances.ListResponse, error) {
	if req.GetPrefix() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "prefix is required")
	}
	q := datastore.NewQuery(model.VMKind).Eq("prefix", req.Prefix)
	rsp := &instances.ListResponse{}
	f := func(vm *model.VM) {
		rsp.Instances = append(rsp.Instances, vmToInstance(vm))
	}
	if err := datastore.Run(c, q, f); err != nil {
		return nil, err
	}
	return rsp, nil
}

// NewInstancesServer returns a new instances server.
func NewInstancesServer() instances.InstancesServer {
	return &instances.DecoratedInstances{
		Prelude:  authPrelude,
		Service:  &Instances{},
		Postlude: gRPCifyAndLogErr,
	}
}
