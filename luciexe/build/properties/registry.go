// Copyright 2024 The LUCI Authors.
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

package properties

import (
	"fmt"
	"reflect"
	"sync"

	structpb "github.com/golang/protobuf/ptypes/struct"

	"go.chromium.org/luci/common/data/stringset"
)

type inputParser func(s *structpb.Struct, target any) error
type outputSerializer func(source any) []byte

type registration struct {
	typIn      reflect.Type
	parseInput inputParser // nil for no input or *structpb.Struct

	typOut          reflect.Type
	serializeOutput outputSerializer // nil for no output or *structpb.Struct

	file string
	line int
}

// Registry contains a mapping for all known property namespaces.
//
// You can add to this registry with Register.
type Registry struct {
	mu sync.Mutex

	final bool

	// This is the registration on a per-namespace basis.
	//
	// Each registration records an input and/or output type and parser/serializer
	// respectively.
	//
	// These are added by the (Must)?Register(In)?(Out)? family of functions.
	regs map[string]registration

	// topLevelFields retains the visible fields for the top-level inupt and
	// output registration (i.e. namespace == "") to check for conflicts.
	topLevelFields stringset.Set

	// topLevelStrict is true if OptStrictTopLevelFields was passed when
	// registering "".
	topLevelStrict bool
}

// registrationInfo is currently just for testing, but this will be expanded
// later to allow a Registry to print out all registered namespaces and details
// about the registration types.
//
// This information will eventually be used to populate a `-help` page for
// luciexe's using the go.chromium.org/luci/luciexe/build library.
type registrationInfo struct {
	InputLocation string
	InputTypeName string

	OutputLocation string
	OutputTypeName string
}

// see comment on registrationInfo
func (r *Registry) listRegistrations() map[string]*registrationInfo {
	r.mu.Lock()
	defer r.mu.Unlock()

	ret := make(map[string]*registrationInfo, len(r.regs))
	for ns, reg := range r.regs {
		itm, ok := ret[ns]
		if !ok {
			itm = &registrationInfo{}
			ret[ns] = itm
		}

		if reg.typIn != nil {
			itm.InputLocation = fmt.Sprintf("%s:%d", reg.file, reg.line)
			itm.InputTypeName = reg.typIn.String()
		}
		if reg.typOut != nil {
			itm.OutputLocation = fmt.Sprintf("%s:%d", reg.file, reg.line)
			itm.OutputTypeName = reg.typOut.String()
		}
	}

	return ret
}
