// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/proto/google/descriptor"
	"github.com/luci/luci-go/common/prpc"
	"github.com/luci/luci-go/server/discovery"
)

var cmdShow = &subcommands.Command{
	UsageLine: `show [flags] <server> [name]

  server: host ("example.com") or port for localhost (":8080").
  name: a full name of service, method or type. If not specified prints names
  	of all available services.`,
	ShortDesc: "lists services and prints a definition of a referenced object.",
	LongDesc:  "Lists services and prints a definition of a referenced object.",
	CommandRun: func() subcommands.CommandRun {
		c := &showRun{}
		c.registerBaseFlags()
		return c
	},
}

type showRun struct {
	cmdRun
}

func (r *showRun) Run(a subcommands.Application, args []string) int {
	if r.cmd == nil {
		r.cmd = cmdShow
	}

	var host, name string
	switch len(args) {
	case 2:
		name = args[1]
		fallthrough
	case 1:
		host = args[0]

	default:
		return r.argErr("")
	}

	client, err := r.authenticatedClient(host)
	if err != nil {
		return 2
	}

	return r.run(func(c context.Context) error {
		return show(c, client, name)
	})
}

// show prints a definition of an object referenced by name in proto3 style.
func show(c context.Context, client *prpc.Client, name string) error {
	desc, err := loadDescription(c, client)
	if err != nil {
		return fmt.Errorf("could not load server description: %s", err)
	}

	if name == "" {
		for _, s := range desc.services {
			fmt.Println(s)
		}
		return nil
	}

	file, obj, path := desc.descriptor.Resolve(name)
	if obj == nil {
		return fmt.Errorf("name %q could not resolved", name)
	}

	print := newPrinter(os.Stdout)
	print.File = file

	switch obj := obj.(type) {

	case *descriptor.ServiceDescriptorProto:
		print.Service(obj, path[1], -1)

	case *descriptor.MethodDescriptorProto:
		serviceIndex, methodIndex := path[1], path[3]
		print.Service(file.Service[serviceIndex], serviceIndex, methodIndex)

		printMsg := func(name string) {
			name = strings.TrimPrefix(name, ".")
			file, msg, path := desc.descriptor.Resolve(name)
			if msg == nil {
				print.Printf("// Message %q is not found\n", name)
				return
			}
			print.File = file
			print.Message(msg.(*descriptor.DescriptorProto), path)
		}

		print.Printf("\n")
		printMsg(obj.GetInputType())
		print.Printf("\n")
		printMsg(obj.GetOutputType())

	case *descriptor.DescriptorProto:
		print.Message(obj, path)

	case *descriptor.FieldDescriptorProto:
		print.Field(obj, path)

	case *descriptor.EnumDescriptorProto:
		print.Enum(obj, path)

	case *descriptor.EnumValueDescriptorProto:
		print.EnumValue(obj, path)

	default:
		return fmt.Errorf("object of type %T is not supported", obj)
	}

	return print.Err
}

type serverDescription struct {
	services   []string
	descriptor *descriptor.FileDescriptorSet
}

func loadDescription(c context.Context, client *prpc.Client) (*serverDescription, error) {
	dc := discovery.NewDiscoveryPRPCClient(client)
	res, err := dc.Describe(c, &discovery.Void{})
	if err != nil {
		return nil, fmt.Errorf("could not load server description: %s", err)
	}

	result := &serverDescription{
		services:   res.Services,
		descriptor: &descriptor.FileDescriptorSet{},
	}
	if err := proto.Unmarshal(res.FileDescriptionSet, result.descriptor); err != nil {
		return nil, fmt.Errorf("could not unmarshal FileDescriptionSet: %s", err)
	}
	return result, nil
}
