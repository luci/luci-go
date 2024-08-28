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
	"fmt"
	"strings"

	"go.starlark.net/starlark"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"
	luciproto "go.chromium.org/luci/common/proto"
	"go.chromium.org/luci/starlark/interpreter"
	"go.chromium.org/luci/starlark/starlarkproto"

	// Dependency of some LUCI protos.
	_ "github.com/envoyproxy/protoc-gen-validate/validate"
	_ "go.chromium.org/luci/buildbucket/proto"
	_ "go.chromium.org/luci/common/proto/config"
	_ "go.chromium.org/luci/common/proto/realms"
	_ "go.chromium.org/luci/cv/api/config/legacy"
	_ "go.chromium.org/luci/cv/api/config/v2"
	_ "go.chromium.org/luci/logdog/api/config/svcconfig"
	_ "go.chromium.org/luci/luci_notify/api/config"
	_ "go.chromium.org/luci/milo/proto/projectconfig"
	_ "go.chromium.org/luci/resultdb/proto/v1"
	_ "go.chromium.org/luci/scheduler/appengine/messages"
	// This covers all of google/api/*.proto
	_ "google.golang.org/genproto/googleapis/api/annotations"
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
	_ "google.golang.org/protobuf/types/known/anypb"
	_ "google.golang.org/protobuf/types/known/durationpb"
	_ "google.golang.org/protobuf/types/known/emptypb"
	_ "google.golang.org/protobuf/types/known/structpb"
	_ "google.golang.org/protobuf/types/known/timestamppb"
	_ "google.golang.org/protobuf/types/known/wrapperspb"
)

// Collection of built-in descriptor sets built from the protobuf registry
// embedded into the lucicfg binary.
var (
	wellKnownDescSet   *starlarkproto.DescriptorSet
	googTypesDescSet   *starlarkproto.DescriptorSet
	annotationsDescSet *starlarkproto.DescriptorSet
	validateDescSet    *starlarkproto.DescriptorSet
	luciTypesDescSet   *starlarkproto.DescriptorSet
)

// init initializes DescSet global vars.
//
// Uses the protobuf registry embedded into the binary. It visits imports in
// topological order, to make sure all cross-file references are correctly
// resolved. We assume there are no circular dependencies (if there are, they'll
// be caught by hanging unit tests).
func init() {
	visited := stringset.New(0)

	// Various well-known proto types (see also starlark/internal/descpb.star).
	wellKnownDescSet = builtinDescriptorSet(
		visited, "google/protobuf", "google/protobuf/",
		[]string{
			"google/protobuf/any.proto",
			"google/protobuf/descriptor.proto",
			"google/protobuf/duration.proto",
			"google/protobuf/empty.proto",
			"google/protobuf/field_mask.proto",
			"google/protobuf/struct.proto",
			"google/protobuf/timestamp.proto",
			"google/protobuf/wrappers.proto",
		})

	// Google API types (see also starlark/internal/descpb.star).
	googTypesDescSet = builtinDescriptorSet(
		visited, "google/type", "google/type/",
		[]string{
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
		}, wellKnownDescSet)

	// Google API annotations (see also starlark/internal/descpb.star).
	annotationsDescSet = builtinDescriptorSet(
		visited, "google/api", "google/api/",
		[]string{
			"google/api/annotations.proto",
			"google/api/client.proto",
			"google/api/field_behavior.proto",
			"google/api/http.proto",
			"google/api/resource.proto",
		}, wellKnownDescSet)

	// github.com/envoyproxy/protoc-gen-validate protos, since they are needed by
	// some LUCI protos (see also starlark/internal/descpb.star).
	validateDescSet = builtinDescriptorSet(
		visited, "protoc-gen-validate", "validate/",
		[]string{
			"validate/validate.proto",
		}, wellKnownDescSet)

	// LUCI protos used by stdlib (see also starlark/internal/luci/descpb.star).
	luciTypesDescSet = builtinDescriptorSet(
		visited, "lucicfg/stdlib", "go.chromium.org/luci/",
		[]string{
			"go.chromium.org/luci/buildbucket/proto/common.proto",
			"go.chromium.org/luci/buildbucket/proto/project_config.proto",
			"go.chromium.org/luci/common/proto/config/project_config.proto",
			"go.chromium.org/luci/common/proto/realms/realms_config.proto",
			"go.chromium.org/luci/cv/api/config/legacy/tricium.proto",
			"go.chromium.org/luci/cv/api/config/v2/config.proto",
			"go.chromium.org/luci/logdog/api/config/svcconfig/project.proto",
			"go.chromium.org/luci/luci_notify/api/config/notify.proto",
			"go.chromium.org/luci/milo/proto/projectconfig/project.proto",
			"go.chromium.org/luci/resultdb/proto/v1/invocation.proto",
			"go.chromium.org/luci/resultdb/proto/v1/predicate.proto",
			"go.chromium.org/luci/scheduler/appengine/messages/config.proto",
		}, wellKnownDescSet, googTypesDescSet, annotationsDescSet, validateDescSet)
}

// builtinDescriptorSet assembles a *DescriptorSet from descriptors embedded
// into the binary in the protobuf registry.
//
// Visits 'files' and all their dependencies (not already visited per 'visited'
// set), adding them in topological order to the new DescriptorSet, updating
// 'visited' along the way.
//
// 'name' and 'deps' are passed verbatim to NewDescriptorSet(...).
//
// For each proto file added to the new descriptor set verifies its filename
// starts with the given 'prefix' to detect if we accidentally pick up
// dependencies that should logically belong to a different descriptor set.
//
// Panics on errors. Built-in descriptors can't be invalid.
func builtinDescriptorSet(visited stringset.Set, name, prefix string, files []string, deps ...*starlarkproto.DescriptorSet) *starlarkproto.DescriptorSet {
	var descs []*descriptorpb.FileDescriptorProto
	for _, f := range files {
		var err error
		if descs, err = visitRegistry(descs, f, visited); err != nil {
			panic(fmt.Errorf("%s: %s", f, err))
		}
	}

	var misplacedFiles []string
	for _, desc := range descs {
		if !strings.HasPrefix(desc.GetName(), prefix) {
			misplacedFiles = append(misplacedFiles, desc.GetName())
		}
	}
	if len(misplacedFiles) > 0 {
		panic(fmt.Errorf("%s: proto dependencies are not under %q: %v", name, prefix, misplacedFiles))
	}

	ds, err := starlarkproto.NewDescriptorSet(name, descs, deps)
	if err != nil {
		panic(err)
	}
	return ds
}

// visitRegistry visits dependencies of 'path', and then 'path' itself.
//
// Appends discovered file descriptors to fds and returns it.
func visitRegistry(fds []*descriptorpb.FileDescriptorProto, path string, visited stringset.Set) ([]*descriptorpb.FileDescriptorProto, error) {
	if !visited.Add(path) {
		return fds, nil // visited it already
	}
	fd, err := protoregistry.GlobalFiles.FindFileByPath(path)
	if err != nil {
		return fds, err
	}
	fdp := protodesc.ToFileDescriptorProto(fd)
	for _, d := range fdp.GetDependency() {
		if fds, err = visitRegistry(fds, d, visited); err != nil {
			return fds, fmt.Errorf("%s: %s", d, err)
		}
	}
	return append(fds, fdp), nil
}

// protoMessageDoc returns the message name and a link to its schema doc.
//
// Extracts it from `option (lucicfg.file_metadata) = {...}` embedded
// into the file descriptor proto.
//
// If there's no documentation, returns two empty strings.
func protoMessageDoc(msg *starlarkproto.Message) (name, doc string) {
	fd := msg.MessageType().Descriptor().ParentFile()
	if fd == nil {
		return "", ""
	}
	opts := fd.Options().(*descriptorpb.FileOptions)
	if opts != nil && proto.HasExtension(opts, luciproto.E_FileMetadata) {
		meta := proto.GetExtension(opts, luciproto.E_FileMetadata).(*luciproto.Metadata)
		if meta.GetDocUrl() != "" {
			return string(msg.MessageType().Descriptor().Name()), meta.GetDocUrl()
		}
	}
	return "", "" // not a public proto
}

// protoCache holds a frozen copy of deserialized proto messages.
//
// Implements starlarkproto.MessageCache.
type protoCache struct {
	interner stringInterner
	cache    map[protoCacheKey]*starlarkproto.Message
	total    int64
	warned   bool
}

type protoCacheKey struct {
	cache string
	body  string
	typ   *starlarkproto.MessageType
}

func newProtoCache(interner stringInterner) protoCache {
	return protoCache{
		interner: interner,
		cache:    map[protoCacheKey]*starlarkproto.Message{},
	}
}

// Fetch returns a previously stored message or (nil, nil) if missing.
func (pc *protoCache) Fetch(th *starlark.Thread, cache, body string, typ *starlarkproto.MessageType) (*starlarkproto.Message, error) {
	return pc.cache[protoCacheKey{cache: cache, body: body, typ: typ}], nil
}

// Store stores a deserialized message.
func (pc *protoCache) Store(th *starlark.Thread, cache, body string, msg *starlarkproto.Message) error {
	if !msg.IsFrozen() {
		panic("can store only frozen messages")
	}

	key := protoCacheKey{
		cache: cache,
		body:  pc.interner.internString(body),
		typ:   msg.MessageType(),
	}
	if _, ok := pc.cache[key]; ok {
		return nil
	}

	pc.cache[key] = msg

	pc.total += int64(len(body))
	if pc.total > 500*1000*1000 && !pc.warned {
		logging.Warningf(interpreter.Context(th),
			"lucicfg internals: proto cache is too large, see https://crbug.com/1382916 if this causes issues like OOMs")
		pc.warned = true
	}

	return nil
}
