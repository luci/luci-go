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

package lucicfg

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"

	proto_v1 "github.com/golang/protobuf/proto"
	descpb_v1 "github.com/golang/protobuf/protoc-gen-go/descriptor"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/proto"

	"go.chromium.org/luci/starlark/protohacks"
	"go.chromium.org/luci/starlark/starlarkproto"

	_ "github.com/golang/protobuf/ptypes/any"
	_ "github.com/golang/protobuf/ptypes/duration"
	_ "github.com/golang/protobuf/ptypes/empty"
	_ "github.com/golang/protobuf/ptypes/struct"
	_ "github.com/golang/protobuf/ptypes/timestamp"
	_ "github.com/golang/protobuf/ptypes/wrappers"

	_ "google.golang.org/genproto/googleapis/type/calendarperiod"
	_ "google.golang.org/genproto/googleapis/type/color"
	_ "google.golang.org/genproto/googleapis/type/date"
	_ "google.golang.org/genproto/googleapis/type/dayofweek"
	_ "google.golang.org/genproto/googleapis/type/expr"
	_ "google.golang.org/genproto/googleapis/type/fraction"
	_ "google.golang.org/genproto/googleapis/type/latlng"
	_ "google.golang.org/genproto/googleapis/type/money"
	_ "google.golang.org/genproto/googleapis/type/postaladdress"
	_ "google.golang.org/genproto/googleapis/type/quaternion"
	_ "google.golang.org/genproto/googleapis/type/timeofday"

	_ "go.chromium.org/luci/buildbucket/proto"
	_ "go.chromium.org/luci/common/proto/config"
	_ "go.chromium.org/luci/common/proto/realms"
	_ "go.chromium.org/luci/cq/api/config/v2"
	_ "go.chromium.org/luci/logdog/api/config/svcconfig"
	_ "go.chromium.org/luci/luci_notify/api/config"
	_ "go.chromium.org/luci/milo/api/config"
	_ "go.chromium.org/luci/resultdb/proto/rpc/v1"
	_ "go.chromium.org/luci/scheduler/appengine/messages"
)

// Collection of built-in descriptor sets built from protobuf v1 registry.
var (
	wellKnownDescSet *starlarkproto.DescriptorSet
	googTypesDescSet *starlarkproto.DescriptorSet
	luciTypesDescSet *starlarkproto.DescriptorSet
)

// init initializes DescSet global vars.
//
// Uses protobuf v1 registry embedded into the binary. It visits imports in
// topological order, to make sure all cross-file references are correctly
// resolved. We assume there are no circular dependencies (if there are, they'll
// be caught by hanging unit tests).
func init() {
	visited := stringset.New(0)

	// Various well-known proto types (see also starlark/internal/descpb.star).
	wellKnownDescSet = builtinDescriptorSet("google/protobuf", []string{
		"google/protobuf/any.proto",
		"google/protobuf/descriptor.proto",
		"google/protobuf/duration.proto",
		"google/protobuf/empty.proto",
		"google/protobuf/field_mask.proto",
		"google/protobuf/struct.proto",
		"google/protobuf/timestamp.proto",
		"google/protobuf/wrappers.proto",
	}, visited)

	// Google API types (see also starlark/internal/descpb.star).
	googTypesDescSet = builtinDescriptorSet("google/type", []string{
		"google/type/calendar_period.proto",
		"google/type/color.proto",
		"google/type/date.proto",
		"google/type/dayofweek.proto",
		"google/type/expr.proto",
		"google/type/fraction.proto",
		"google/type/latlng.proto",
		"google/type/money.proto",
		"google/type/postal_address.proto",
		"google/type/quaternion.proto",
		"google/type/timeofday.proto",
	}, visited, wellKnownDescSet)

	// LUCI protos used by stdlib (see also starlark/internal/luci/descpb.star).
	luciTypesDescSet = builtinDescriptorSet("lucicfg/stdlib", []string{
		"go.chromium.org/luci/buildbucket/proto/common.proto",
		"go.chromium.org/luci/buildbucket/proto/project_config.proto",
		"go.chromium.org/luci/common/proto/config/project_config.proto",
		"go.chromium.org/luci/common/proto/realms/realms_config.proto",
		"go.chromium.org/luci/cq/api/config/v2/cq.proto",
		"go.chromium.org/luci/logdog/api/config/svcconfig/project.proto",
		"go.chromium.org/luci/luci_notify/api/config/notify.proto",
		"go.chromium.org/luci/milo/api/config/project.proto",
		"go.chromium.org/luci/resultdb/proto/rpc/v1/invocation.proto",
		"go.chromium.org/luci/resultdb/proto/rpc/v1/predicate.proto",
		"go.chromium.org/luci/scheduler/appengine/messages/config.proto",
	}, visited, wellKnownDescSet, googTypesDescSet)
}

// builtinDescriptorSet assembles a *DescriptorSet from descriptors embedded
// into the binary in protobuf v1 registry.
//
// Visits 'files' and all their dependencies (not already visited per 'visited'
// set), adding them in topological order to the new DescriptorSet, updating
// 'visited' along the way.
//
// 'name' and 'deps' are passed verbatim to NewDescriptorSet(...).
//
// Panics on errors. Built-in descriptors can't be invalid.
func builtinDescriptorSet(name string, files []string, visited stringset.Set, deps ...*starlarkproto.DescriptorSet) *starlarkproto.DescriptorSet {
	list := protohacks.FileDescriptorsList{}
	for _, f := range files {
		if err := visitRegistry(&list, f, visited); err != nil {
			panic(fmt.Errorf("%s: %s", f, err))
		}
	}
	ds, err := starlarkproto.NewDescriptorSet(name, list.Descriptors, deps)
	if err != nil {
		panic(err)
	}
	return ds
}

// visitRegistry visits dependencies of 'path', and then 'path' itself.
func visitRegistry(l *protohacks.FileDescriptorsList, path string, visited stringset.Set) error {
	if !visited.Add(path) {
		return nil // visited it already
	}
	raw, err := rawFileDescriptor(path)
	if err != nil {
		return err
	}
	fd, err := protohacks.UnmarshalFileDescriptorProto(raw)
	if err != nil {
		return err
	}
	for _, d := range fd.GetDependency() {
		if err := visitRegistry(l, d, visited); err != nil {
			return fmt.Errorf("%s: %s", d, err)
		}
	}
	l.Add(fd)
	return nil
}

// rawFileDescriptor extracts raw FileDescriptor from protobuf v1 registry.
func rawFileDescriptor(path string) ([]byte, error) {
	gzblob := proto_v1.FileDescriptor(path)
	if gzblob == nil {
		return nil, fmt.Errorf("proto %q is not in the registry", path)
	}

	r, err := gzip.NewReader(bytes.NewReader(gzblob))
	if err != nil {
		return nil, fmt.Errorf("failed to open gzip reader for %q - %s", path, err)
	}
	defer r.Close()

	b, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to uncompress descriptor for %q - %s", path, err)
	}
	return b, nil
}

// protoMessageDoc returns the message name and a link to its schema doc.
//
// If there's no documentation, returns two empty strings.
func protoMessageDoc(msg *starlarkproto.Message) (name, doc string) {
	fd := msg.MessageType().Descriptor().ParentFile()
	if fd == nil {
		return "", ""
	}
	// Try to grab doc_url from `option (lucicfg.file_metadata) = {...}` embedded
	// into the proto descriptor. Since we still use proto v1 as our *.go code
	// generator, but proto v2 as our runtime API, there are some interoperability
	// issues we solve by round-tripping descriptorpb.FileOptions message through
	// proto serialization (we serialize it as v2 proto, and deserialize it as v1
	// proto, which allows us to use code-generated proto extensions API).
	//
	// If something fails, just give up, it's not a crucial functionality. Note
	// that unit tests verify the golden path.
	if blob, _ := protohacks.FileOptions(fd); blob != nil {
		// TODO(vadimsh): Move this to common/proto.
		fileOpts := &descpb_v1.FileOptions{}
		if err := proto_v1.Unmarshal(blob, fileOpts); err == nil {
			exts, err := proto_v1.GetExtensions(fileOpts, []*proto_v1.ExtensionDesc{proto.E_FileMetadata})
			if err == nil && exts[0] != nil {
				if meta := exts[0].(*proto.Metadata); meta.GetDocUrl() != "" {
					return string(msg.MessageType().Descriptor().Name()), meta.GetDocUrl()
				}
			}
		}
	}
	return "", "" // not a public proto
}
