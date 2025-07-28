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
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/maruel/subcommands"
	"google.golang.org/protobuf/types/descriptorpb"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/proto/google/descutil"
	"go.chromium.org/luci/common/proto/google/descutil/printer"

	"go.chromium.org/luci/grpc/prpc"
)

const (
	cmdShowUsage = `show [flags] <server> [name]

  server: host ("example.com") or port for localhost (":8080").
  name: a full name of service, method or type. If not specified prints names
    of all available services.
`

	cmdShowDesc = "lists services and prints a definition of a referenced object."
)

func cmdShow(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: cmdShowUsage,
		ShortDesc: cmdShowDesc,
		LongDesc:  "Lists services and prints a definition of a referenced object.",
		CommandRun: func() subcommands.CommandRun {
			c := &showRun{}
			c.registerBaseFlags(defaultAuthOpts)
			return c
		},
	}
}

type showRun struct {
	cmdRun
}

func (r *showRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	var host, name string
	switch len(args) {
	case 2:
		name = args[1]
		fallthrough
	case 1:
		host = args[0]

	default:
		return r.argErr(cmdShowDesc, cmdShowUsage, "")
	}

	ctx := cli.GetContext(a, r, env)
	client, err := r.authenticatedClient(ctx, host)
	if err != nil {
		return ecAuthenticatedClientError
	}
	return r.done(show(ctx, client, name))
}

// show prints a definition of an object referenced by name in proto3 style.
func show(ctx context.Context, client *prpc.Client, name string) error {
	desc, err := loadDescription(ctx, client)
	if err != nil {
		return fmt.Errorf("could not load server description: %s", err)
	}

	if name == "" {
		for _, s := range desc.Services {
			fmt.Println(s)
		}
		return nil
	}

	file, obj, path := descutil.Resolve(desc.Description, name)
	if obj == nil {
		return fmt.Errorf("name %q could not resolved", name)
	}

	printer := printer.NewPrinter(os.Stdout)
	if err := printer.SetFile(file); err != nil {
		return err
	}

	switch obj := obj.(type) {
	case *descriptorpb.ServiceDescriptorProto:
		printer.Service(obj, -1)

	case *descriptorpb.MethodDescriptorProto:
		serviceIndex, methodIndex := path[1], path[3]
		printer.Service(file.Service[serviceIndex], methodIndex)

		printMsg := func(name string) error {
			name = strings.TrimPrefix(name, ".")
			file, msg, _ := descutil.Resolve(desc.Description, name)
			if msg == nil {
				printer.Printf("// Message %q is not found\n", name)
				return nil
			}
			if err := printer.SetFile(file); err != nil {
				return err
			}
			printer.Message(msg.(*descriptorpb.DescriptorProto))
			return nil
		}

		printer.Printf("\n")
		if err := printMsg(obj.GetInputType()); err != nil {
			return err
		}
		printer.Printf("\n")
		if err := printMsg(obj.GetOutputType()); err != nil {
			return err
		}

	case *descriptorpb.DescriptorProto:
		printer.Message(obj)

	case *descriptorpb.FieldDescriptorProto:
		printer.Field(obj)

	case *descriptorpb.EnumDescriptorProto:
		printer.Enum(obj)

	case *descriptorpb.EnumValueDescriptorProto:
		printer.EnumValue(obj)

	default:
		return fmt.Errorf("object of type %T is not supported", obj)
	}

	return printer.Err
}
