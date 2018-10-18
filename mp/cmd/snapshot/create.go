// Copyright 2018 The LUCI Authors.
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
	"strings"
	"time"

	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag"
	"go.chromium.org/luci/common/logging"
)

// createSnapshotCmd is the command to create a disk snapshot.
type createSnapshotCmd struct {
	cmdRunBase
	// disk is the name of the disk to create a snapshot of.
	disk string
	// labels is a slice of strings in key:value form to attach to the snapshot as labels.
	labels []string
	// name is the name to give the created snapshot.
	name string
	// project is the name of the project where the disk exists, and to create the snapshot in.
	project string
	// zone is the name of the zone where the disk exists.
	zone string
}

// validateFlags validates parsed command line flags and returns a map of parsed labels.
func (cmd *createSnapshotCmd) validateFlags(c context.Context) (map[string]string, error) {
	if cmd.disk == "" {
		return nil, errors.New("-disk is required")
	}
	labels := make(map[string]string, len(cmd.labels))
	for _, label := range cmd.labels {
		parts := strings.SplitN(label, ":", 2)
		if len(parts) != 2 {
			return nil, errors.New(fmt.Sprintf("-label %q must be in key:value form", label))
		}
		if _, ok := labels[parts[0]]; ok {
			return nil, errors.New(fmt.Sprintf("-label %q has duplicate key", label))
		}
		labels[parts[0]] = parts[1]
	}
	if cmd.name == "" {
		return nil, errors.New("-name is required")
	}
	if cmd.project == "" {
		return nil, errors.New("-project is required")
	}
	if cmd.zone == "" {
		return nil, errors.New("-zone is required")
	}
	return labels, nil
}

// Run runs the command to create a disk snapshot.
func (cmd *createSnapshotCmd) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	c := cli.GetContext(app, cmd, env)
	labels, err := cmd.validateFlags(c)
	if err != nil {
		logging.Errorf(c, "%s", err.Error())
		return 1
	}

	srv := getService(c)
	logging.Infof(c, "Creating snapshot.")
	snapshot := &compute.Snapshot{
		Labels: labels,
		Name:   cmd.name,
	}
	op, err := compute.NewDisksService(srv).CreateSnapshot(cmd.project, cmd.zone, cmd.disk, snapshot).Context(c).Do()
	for {
		if err != nil {
			for _, err := range err.(*googleapi.Error).Errors {
				logging.Errorf(c, "%s", err.Message)
			}
			return 1
		}
		if op.Status == "DONE" {
			break
		}
		logging.Infof(c, "Waiting for snapshot to be created...")
		time.Sleep(2 * time.Second)
		op, err = compute.NewZoneOperationsService(srv).Get(cmd.project, cmd.zone, op.Name).Context(c).Do()
	}
	if op.Error != nil {
		for _, err := range op.Error.Errors {
			logging.Errorf(c, "%s", err.Message)
		}
		return 1
	}
	logging.Infof(c, "Created snapshot.")
	return 0
}

// getCreateSnapshotCmd returns a new command to create a snapshot.
func getCreateSnapshotCmd() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "create -disk <disk> -name <name> -project <project> -zone <zone> [-label <key>:<value>]...",
		ShortDesc: "creates a snapshot",
		LongDesc:  "Creates a disk snapshot.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &createSnapshotCmd{}
			cmd.Initialize()
			cmd.Flags.StringVar(&cmd.disk, "disk", "", "Disk to create a snapshot of.")
			cmd.Flags.Var(flag.StringSlice(&cmd.labels), "label", "Label to attach to a snapshot.")
			cmd.Flags.StringVar(&cmd.name, "name", "", "Name to give the created snapshot.")
			cmd.Flags.StringVar(&cmd.project, "project", "", "Project where the disk exists, and to create the snapshot in.")
			cmd.Flags.StringVar(&cmd.zone, "zone", "", "Zone where the disk exists.")
			return cmd
		},
	}
}
