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

// Package main contains a tool which takes a snapshot of the given disk.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/common/errors"
	luciflag "go.chromium.org/luci/common/flag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

// flags encapsulates the command line flags used to invoke this tool.
type flags struct {
	// authOpts encapsulates the parsed auth options.
	authOpts auth.Options
	// disk is the name of the disk to create a snapshot of.
	disk string
	// labels is a map to attach to the snapshot as labels.
	labels map[string]string
	// name is the name to give the created snapshot.
	name string
	// project is the name of the project where the disk exists, and to create the snapshot in.
	project string
	// zone is the name of the zone where the disk exists.
	zone string
}

// parseFlags parses command line flags, returning a flags struct.
func parseFlags(c context.Context, args []string) (*flags, error) {
	a := authcli.Flags{}
	s := &flag.FlagSet{}
	opts := chromeinfra.DefaultAuthOptions()
	opts.Scopes = []string{"https://www.googleapis.com/auth/compute"}
	a.Register(s, opts)

	var labels []string
	f := flags{}
	s.StringVar(&f.disk, "disk", "", "Disk to create a snapshot of.")
	s.Var(luciflag.StringSlice(&labels), "label", "Label to attach to a snapshot.")
	s.StringVar(&f.name, "name", "", "Name to give the created snapshot.")
	s.StringVar(&f.project, "project", "", "Project where the disk exists, and to create the snapshot in.")
	s.StringVar(&f.zone, "zone", "", "Zone where the disk exists.")
	if err := s.Parse(args); err != nil {
		return nil, err
	}

	if f.disk == "" {
		return nil, errors.New("-disk is required")
	}
	f.labels = make(map[string]string, len(labels))
	for _, label := range labels {
		parts := strings.SplitN(label, ":", 2)
		if len(parts) != 2 {
			return nil, errors.New(fmt.Sprintf("-label %q must be in key:value form", label))
		}
		if _, ok := f.labels[parts[0]]; ok {
			return nil, errors.New(fmt.Sprintf("-label %q has duplicate key", label))
		}
		f.labels[parts[0]] = parts[1]
	}
	if f.name == "" {
		return nil, errors.New("-name is required")
	}
	if f.project == "" {
		return nil, errors.New("-project is required")
	}
	if f.zone == "" {
		return nil, errors.New("-zone is required")
	}
	opts, err := a.Options()
	if err != nil {
		return nil, err
	}
	f.authOpts = opts
	return &f, nil
}

// getClient returns a new Compute Engine API client.
func getClient(c context.Context, opts auth.Options) (*compute.Service, error) {
	http, err := auth.NewAuthenticator(c, auth.OptionalLogin, opts).Client()
	if err != nil {
		return nil, err
	}
	service, err := compute.New(http)
	if err != nil {
		return nil, err
	}
	return service, nil
}

// Main creates a disk snapshot.
func Main(args []string) int {
	c := gologger.StdConfig.Use(context.Background())
	c = logging.SetLevel(c, logging.Debug)

	flags, err := parseFlags(c, args)
	if err != nil {
		logging.Errorf(c, "%s", err.Error())
		return 1
	}

	service, err := getClient(c, flags.authOpts)
	if err != nil {
		logging.Errorf(c, "%s", err.Error())
		return 1
	}

	logging.Infof(c, "Creating snapshot.")
	snapshot := &compute.Snapshot{
		Labels: flags.labels,
		Name:   flags.name,
	}
	op, err := compute.NewDisksService(service).CreateSnapshot(flags.project, flags.zone, flags.disk, snapshot).Context(c).Do()
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
		op, err = compute.NewZoneOperationsService(service).Get(flags.project, flags.zone, op.Name).Context(c).Do()
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

func main() {
	os.Exit(Main(os.Args[1:]))
}
