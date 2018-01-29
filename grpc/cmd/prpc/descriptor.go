// Copyright 2016 The LUCI Authors.
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

package main

import (
	"fmt"
	"strings"

	"go.chromium.org/luci/common/proto/google/descutil"

	"golang.org/x/net/context"

	"github.com/golang/protobuf/protoc-gen-go/descriptor"
	"go.chromium.org/luci/grpc/discovery"
	"go.chromium.org/luci/grpc/prpc"
)

type serverDescription struct {
	*discovery.DescribeResponse
}

func loadDescription(c context.Context, client *prpc.Client) (*serverDescription, error) {
	dc := discovery.NewDiscoveryPRPCClient(client)
	res, err := dc.Describe(c, &discovery.Void{})
	if err != nil {
		return nil, fmt.Errorf("could not load server description: %s", err)
	}

	return &serverDescription{res}, nil
}

// resolveInputMessage resolves input message type of a method.
func (d *serverDescription) resolveInputMessage(service, method string) (*descriptor.DescriptorProto, error) {
	_, obj, _ := descutil.Resolve(d.Description, service)
	serviceDesc, ok := obj.(*descriptor.ServiceDescriptorProto)
	if !ok {
		return nil, fmt.Errorf("service %q not found", service)
	}

	mi := descutil.FindMethodForService(serviceDesc, method)
	if mi == -1 {
		return nil, fmt.Errorf("method %q in service %q not found", method, service)
	}

	msgName := serviceDesc.Method[mi].GetInputType()
	msgName = strings.TrimPrefix(msgName, ".")
	return d.resolveMessage(msgName)
}

func (d *serverDescription) resolveMessage(name string) (*descriptor.DescriptorProto, error) {
	_, obj, _ := descutil.Resolve(d.Description, name)
	msg, ok := obj.(*descriptor.DescriptorProto)
	if !ok {
		return nil, fmt.Errorf("message %q not found", name)
	}
	return msg, nil
}
