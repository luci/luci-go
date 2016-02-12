// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"fmt"
	"strings"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/proto/google/descriptor"
	"github.com/luci/luci-go/common/prpc"
	"github.com/luci/luci-go/server/discovery"
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
	_, obj, _ := d.Description.Resolve(service)
	serviceDesc, ok := obj.(*descriptor.ServiceDescriptorProto)
	if !ok {
		return nil, fmt.Errorf("service %q not found", service)
	}

	mi := serviceDesc.FindMethod(method)
	if mi == -1 {
		return nil, fmt.Errorf("method %q in service %q not found", method, service)
	}

	msgName := serviceDesc.Method[mi].GetInputType()
	msgName = strings.TrimPrefix(msgName, ".")
	return d.resolveMessage(msgName)
}

func (d *serverDescription) resolveMessage(name string) (*descriptor.DescriptorProto, error) {
	_, obj, _ := d.Description.Resolve(name)
	msg, ok := obj.(*descriptor.DescriptorProto)
	if !ok {
		return nil, fmt.Errorf("message %q not found", name)
	}
	return msg, nil
}
