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

package cli

import (
	"flag"
	"fmt"
	"sort"
	"strings"

	pb "go.chromium.org/luci/buildbucket/proto"
)

var statusFlagValues = map[string]pb.Status{
	"scheduled":     pb.Status_SCHEDULED,
	"started":       pb.Status_STARTED,
	"ended":         pb.Status_ENDED_MASK,
	"success":       pb.Status_SUCCESS,
	"failure":       pb.Status_FAILURE,
	"infra_failure": pb.Status_INFRA_FAILURE,
	"canceled":      pb.Status_CANCELED,
}

var statusFlagNames map[pb.Status]string
var statusFlagValuesName []string

func init() {
	statusFlagNames = make(map[pb.Status]string, len(statusFlagValues))
	statusFlagValuesName = make([]string, 0, len(statusFlagValues))
	for name, status := range statusFlagValues {
		statusFlagValuesName = append(statusFlagValuesName, name)
		statusFlagNames[status] = name
	}
	sort.Strings(statusFlagValuesName)
}

type statusFlag struct {
	status *pb.Status
}

// StatusFlag returns a flag.Getter which reads a flag value into status.
// Valid flag values: scheduled, started, ended, success, failure, infra_failure,
// canceled.
// Panics if status is nil.
func StatusFlag(status *pb.Status) flag.Getter {
	if status == nil {
		panic("status is nil")
	}
	return &statusFlag{status}
}

func (f *statusFlag) String() string {
	// https://godoc.org/flag#Value says that String() may be called with a
	// zero-valued receiver.
	if f == nil || f.status == nil {
		return ""
	}
	return statusFlagNames[*f.status]
}

func (f *statusFlag) Get() interface{} {
	return *f.status
}

func (f *statusFlag) Set(s string) error {
	st, ok := statusFlagValues[strings.ToLower(s)]
	if !ok {
		return fmt.Errorf("invalid status %q; expected one of %s", s, strings.Join(statusFlagValuesName, ", "))
	}

	*f.status = pb.Status(st)
	return nil
}
