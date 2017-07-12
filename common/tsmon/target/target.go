// Copyright 2015 The LUCI Authors.
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

// Package target contains information about the thing that is sending metrics -
// either a NetworkDevice (a machine) or a Task (a service).
// There is a default target that is usually configured with commandline flags
// (flags.go), but a target can also be passed through the Context (context.go)
// if you need to set metric values for a different target.
package target

import (
	"errors"
	"fmt"
	"hash/fnv"

	"github.com/luci/luci-go/common/tsmon/types"
)

// A Task is a process or a service running on one or more machine.
type Task struct {
	ServiceName string
	JobName     string
	DataCenter  string
	HostName    string
	TaskNum     int32
}

// Hash returns a uint64 hash of this target.
func (t *Task) Hash() uint64 {
	h := fnv.New64a()
	fmt.Fprintf(h, "%s\n%s\n%s\n%s\n%d",
		t.ServiceName,
		t.JobName,
		t.DataCenter,
		t.HostName,
		t.TaskNum)
	return h.Sum64()
}

// Clone returns a copy of this object.
func (t *Task) Clone() types.Target {
	clone := *t
	return &clone
}

// A NetworkDevice is a machine that has a hostname.
type NetworkDevice struct {
	Metro     string
	Role      string
	Hostname  string
	Hostgroup string
}

// Hash returns a uint64 hash of this target.
func (t *NetworkDevice) Hash() uint64 {
	h := fnv.New64a()
	fmt.Fprintf(h, "%s\n%s\n%s\n%s",
		t.Metro,
		t.Role,
		t.Hostname,
		t.Hostgroup)
	return h.Sum64()
}

// Clone returns a copy of this object.
func (t *NetworkDevice) Clone() types.Target {
	clone := *t
	return &clone
}

// NewFromFlags returns a Target configured from commandline flags.
func NewFromFlags(fl *Flags) (types.Target, error) {
	if fl.TargetType == "task" {
		if fl.TaskServiceName == "" {
			return nil, errors.New(
				"--ts-mon-task-service-name must be provided when using --ts-mon-target-type=task")
		}
		if fl.TaskJobName == "" {
			return nil, errors.New(
				"--ts-mon-task-job-name must be provided when using --ts-mon-target-type=task")
		}

		return &Task{
			ServiceName: fl.TaskServiceName,
			JobName:     fl.TaskJobName,
			DataCenter:  fl.TaskRegion,
			HostName:    fl.TaskHostname,
			TaskNum:     int32(fl.TaskNumber),
		}, nil
	} else if fl.TargetType == "device" {
		return &NetworkDevice{
			Metro:     fl.DeviceRegion,
			Role:      fl.DeviceRole,
			Hostname:  fl.DeviceHostname,
			Hostgroup: fl.DeviceNetwork,
		}, nil
	} else {
		return nil, fmt.Errorf("unknown --ts-mon-target-type '%s'", fl.TargetType)
	}
}
