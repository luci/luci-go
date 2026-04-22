// Copyright 2026 The LUCI Authors.
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

package cli

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"

	"go.chromium.org/luci/cipd/client/cipd"
)

////////////////////////////////////////////////////////////////////////////////
// 'describe' subcommand.

func cmdDescribe(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "describe <package> [options]",
		ShortDesc: "returns information about a package instance given its version",
		LongDesc: "Returns information about a package instance given its version: " +
			"who uploaded the instance and when and a list of attached tags.",
		CommandRun: func() subcommands.CommandRun {
			c := &describeRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.Flags.StringVar(&c.version, "version", "<version>", "Package version to describe.")
			return c
		},
	}
}

type describeRun struct {
	cipdSubcommand
	clientOptions

	version string
}

func isPrintableContentType(contentType string) bool {
	if strings.HasPrefix(contentType, "text/") ||
		slices.Contains([]string{"application/json", "application/jwt"}, contentType) {
		return true
	}
	return false
}

func (c *describeRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)
	return c.done(describeInstance(ctx, args[0], c.version, c.clientOptions))
}

func describeInstance(ctx context.Context, pkg, version string, clientOpts clientOptions) (*cipd.InstanceDescription, error) {
	pkg, err := expandTemplate(pkg)
	if err != nil {
		return nil, err
	}

	client, err := clientOpts.makeCIPDClient(ctx, "")
	if err != nil {
		return nil, err
	}
	defer client.Close(ctx)

	pin, err := client.ResolveVersion(ctx, pkg, version)
	if err != nil {
		return nil, err
	}

	desc, err := client.DescribeInstance(ctx, pin, &cipd.DescribeInstanceOpts{
		DescribeRefs:     true,
		DescribeTags:     true,
		DescribeMetadata: true,
	})
	if err != nil {
		return nil, err
	}

	fmt.Printf("Package:       %s\n", desc.Pin.PackageName)
	fmt.Printf("Instance ID:   %s\n", desc.Pin.InstanceID)
	fmt.Printf("Registered by: %s\n", desc.RegisteredBy)
	fmt.Printf("Registered at: %s\n", time.Time(desc.RegisteredTs).Local())
	if len(desc.Refs) != 0 {
		fmt.Printf("Refs:\n")
		for _, t := range desc.Refs {
			fmt.Printf("  %s\n", t.Ref)
		}
	} else {
		fmt.Printf("Refs:          none\n")
	}
	if len(desc.Tags) != 0 {
		fmt.Printf("Tags:\n")
		for _, t := range desc.Tags {
			fmt.Printf("  %s\n", t.Tag)
		}
	} else {
		fmt.Printf("Tags:          none\n")
	}
	if len(desc.Metadata) != 0 {
		fmt.Printf("Metadata:\n")
		for _, md := range desc.Metadata {
			printValue := string(md.Value)
			if !isPrintableContentType(md.ContentType) {
				// Content type is highly unlikely to be meaningfully printable.
				// Output something reasonable to the CLI (JSON will still have
				// the full payload).
				printValue = fmt.Sprintf("<%s binary, %d bytes>", md.ContentType, len(md.Value))
			}
			// Add indentation if the value contains newlines.
			if strings.Contains(printValue, "\n") {
				printValue = "\n" + printValue
				printValue = strings.ReplaceAll(printValue, "\n", "\n    ")
			}
			fmt.Printf("  %s:%s\n", md.Key, printValue)
		}
	} else {
		fmt.Printf("Metadata:      none\n")
	}

	return desc, nil
}
