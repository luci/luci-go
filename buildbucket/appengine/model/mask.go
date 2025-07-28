// Copyright 2021 The LUCI Authors.
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

package model

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/common/proto/structmask"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// The default field mask to use for read requests.
var defaultFieldMask = fieldmaskpb.FieldMask{
	Paths: []string{
		"builder",
		"canary",
		"create_time",
		"created_by",
		"critical",
		"end_time",
		"id",
		"input.experimental",
		"input.gerrit_changes",
		"input.gitiles_commit",
		"number",
		"start_time",
		"status",
		"status_details",
		"update_time",
	},
}

// Used just for their type information.
var (
	buildPrototype       = pb.Build{}
	searchBuildPrototype = pb.SearchBuildsResponse{}
)

// NoopBuildMask selects all fields.
var NoopBuildMask = &BuildMask{m: mask.All(&buildPrototype)}

// DefaultBuildMask is the default mask to use for read requests.
var DefaultBuildMask = HardcodedBuildMask(defaultFieldMask.Paths...)

// ListOnlyBuildMask is an extra mask to hide fields from callers who have the BuildsList
// permission but not BuildsGet or BuildsGetLimited.
// These callers should only be able to see fields specified in this mask.
var ListOnlyBuildMask = HardcodedBuildMask(BuildFieldsWithVisibility(pb.BuildFieldVisibility_BUILDS_LIST_PERMISSION)...)

// GetLimitedBuildMask is an extra mask to hide fields from callers who have the BuildsGetLimited
// permission but not BuildsGet.
// These callers should only be able to see fields specified in this mask.
var GetLimitedBuildMask = HardcodedBuildMask(BuildFieldsWithVisibility(pb.BuildFieldVisibility_BUILDS_GET_LIMITED_PERMISSION)...)

// BuildMask knows how to filter pb.Build proto messages.
type BuildMask struct {
	m            *mask.Mask             // the overall field mask
	in           *structmask.Filter     // "input.properties" filter
	out          *structmask.Filter     // "output.properties" filter
	req          *structmask.Filter     // "infra.buildbucket.requested_properties" filter
	stepStatuses map[pb.Status]struct{} // "steps.status" filter
	allFields    bool                   // Flag for including all fields.
}

// NewBuildMask constructs a build mask either using a legacy `fields` FieldMask
// or new `mask` BuildMask (but not both at the same time, pick one).
//
// legacyPrefix is usually "", but can be "builds" to trim "builds." from
// the legacy field mask (used by SearchBuilds API).
//
// If the mask is empty, returns DefaultBuildMask.
func NewBuildMask(legacyPrefix string, legacy *fieldmaskpb.FieldMask, bm *pb.BuildMask) (*BuildMask, error) {
	switch {
	case legacy == nil && bm == nil:
		return DefaultBuildMask, nil
	case legacy != nil && bm != nil:
		return nil, errors.New("`mask` and `fields` can't be used together, prefer `mask` since `fields` is deprecated")
	case legacy != nil:
		return newLegacyBuildMask(legacyPrefix, legacy)
	}

	// Filter unique statuses.
	stepStatuses := make(map[pb.Status]struct{}, len(pb.Status_name))
	for _, st := range bm.StepStatus {
		stepStatuses[st] = struct{}{}
	}

	if bm.GetAllFields() {
		// All fields should be included.
		if len(bm.GetFields().GetPaths()) > 0 || len(bm.GetInputProperties()) > 0 || len(bm.GetOutputProperties()) > 0 || len(bm.GetRequestedProperties()) > 0 {
			return nil, errors.New("mask.AllFields is mutually exclusive with other mask fields")
		}
		return &BuildMask{allFields: true, stepStatuses: stepStatuses}, nil
	}

	fm := bm.Fields
	if len(fm.GetPaths()) == 0 {
		fm = &defaultFieldMask
	}

	var cloned bool
	structFilter := func(path string, structMask []*structmask.StructMask) (*structmask.Filter, error) {
		if len(structMask) == 0 {
			return nil, nil
		}
		// Implicitly include struct-valued fields when their masks are present.
		// Make sure not to accidentally override the original FieldMask
		// (in particular when it is &defaultFieldMask).
		if !cloned {
			fm = proto.Clone(fm).(*fieldmaskpb.FieldMask)
			cloned = true
		}
		fm.Paths = append(fm.Paths, path)
		return structmask.NewFilter(structMask)
	}

	// Parse struct masks. This also mutates `fm` to include corresponding fields.
	in, err := structFilter("input.properties", bm.InputProperties)
	if err != nil {
		return nil, errors.Fmt(`bad "input_properties" struct mask: %w`, err)
	}
	out, err := structFilter("output.properties", bm.OutputProperties)
	if err != nil {
		return nil, errors.Fmt(`bad "output_properties" struct mask: %w`, err)
	}
	req, err := structFilter("infra.buildbucket.requested_properties", bm.RequestedProperties)
	if err != nil {
		return nil, errors.Fmt(`bad "requested_properties" struct mask: %w`, err)
	}

	// Construct the overall pb.Build mask.
	var m *mask.Mask
	if fm == &defaultFieldMask {
		// An optimization for the common case, to avoid constructing mask.Mask all
		// the time.
		m = DefaultBuildMask.m
	} else {
		var err error
		if m, err = mask.FromFieldMask(fm, &buildPrototype, mask.AdvancedSemantics()); err != nil {
			return nil, err
		}
	}

	// We want to support only field masks compatible with Go protobuf library.
	// Note that "go.chromium.org/luci/common/proto/mask" implements a superset
	// of this functionality. It also returns detailed errors. So we used it first
	// to reject obviously invalid masks (e.g. referring to unknown fields) with
	// nice error messages, and use a blunt IsValid check below to reject no
	// longer supported non-protobuf compatible masks.
	if !fm.IsValid(&buildPrototype) {
		return nil, errors.New("the extended field mask syntax is no longer supported, " +
			"use the standard one: " +
			"https://pkg.go.dev/google.golang.org/protobuf/types/known/fieldmaskpb#FieldMask")
	}

	return &BuildMask{
		m:            m,
		in:           in,
		out:          out,
		req:          req,
		stepStatuses: stepStatuses,
	}, nil
}

// newLegacyBuildMask constructs BuildMask from legacy `fields` field.
func newLegacyBuildMask(legacyPrefix string, fields *fieldmaskpb.FieldMask) (*BuildMask, error) {
	if len(fields.GetPaths()) == 0 {
		return DefaultBuildMask, nil
	}
	var m *mask.Mask
	var err error
	switch legacyPrefix {
	case "":
		m, err = mask.FromFieldMask(fields, &buildPrototype, mask.AdvancedSemantics())
	case "builds":
		m, err = mask.FromFieldMask(fields, &searchBuildPrototype, mask.AdvancedSemantics())
		if err == nil {
			m, err = m.Submask("builds.*")
		}
	default:
		panic(fmt.Sprintf("unsupported legacy prefix %q", legacyPrefix))
	}
	if err != nil {
		return nil, err
	}
	return &BuildMask{m: m}, nil
}

// HardcodedBuildMask returns a build mask with given fields.
//
// Panics if some of them are invalid. Intended to be used to initialize
// constants or in tests.
func HardcodedBuildMask(fields ...string) *BuildMask {
	return &BuildMask{m: mask.MustFromReadMask(&buildPrototype, fields...)}
}

// BuildFieldsWithVisibility returns a list of Build fields that are visible
// with the specified level of read permission. For example, the following:
//
//	BuildFieldsWithVisibility(pb.BuildFieldVisibility_BUILDS_GET_LIMITED_PERMISSION)
//
// will return a list of Build fields (including nested fields) that have been
// annotated with either of the following field options:
//
//	[(visible_with) = BUILDS_GET_LIMITED_PERMISSION]
//	[(visible_with) = BUILDS_LIST_PERMISSION]
//
// Note that visibility permissions are strictly ordered: if a user has the
// GetLimited permission, that implies they also have the List permission.
func BuildFieldsWithVisibility(visibility pb.BuildFieldVisibility) []string {
	paths := make([]string, 0, 16)
	findFieldPathsWithVisibility(buildPrototype.ProtoReflect().Descriptor(), []string{}, visibility, &paths)
	return paths
}

func findFieldPathsWithVisibility(md protoreflect.MessageDescriptor, path []string, visibility pb.BuildFieldVisibility, outPaths *[]string) {
	fields := md.Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		name := string(fd.Name())
		opts := fd.Options().(*descriptorpb.FieldOptions)
		fieldVisibility := proto.GetExtension(opts, pb.E_VisibleWith).(pb.BuildFieldVisibility)
		if fieldVisibility.Number() >= visibility.Number() {
			*outPaths = append(*outPaths, strings.Join(append(path, name), "."))
		}
		// Simplifying hack: since we currently only need recursion to depth 1,
		// don't recurse into child messages if there is any path prefix.
		// This allows us to avoid implementing cycle detection.
		// If, in future, we want to give extended access to fields nested more
		// than 1 message deep, this hack will need to be extended.
		// Since field visibility fails closed, this isn't a security risk.
		if len(path) > 0 {
			continue
		}
		if fd.Kind() == protoreflect.MessageKind {
			findFieldPathsWithVisibility(fd.Message(), append(path, name), visibility, outPaths)
		}
	}
}

// Includes returns true if the given field path is included in the mask
// (either partially or entirely), or the mask includes all fields.
//
// Panics if the fieldPath is invalid.
func (m *BuildMask) Includes(fieldPath string) bool {
	if m.allFields {
		return true
	}
	inc, err := m.m.Includes(fieldPath)
	if err != nil {
		panic(errors.Fmt("bad field path %q: %w", fieldPath, err))
	}
	return inc != mask.Exclude
}

// Trim applies the mask to the build in-place.
func (m *BuildMask) Trim(b *pb.Build) error {
	if err := m.m.Trim(b); err != nil {
		return err
	}
	if m.in != nil && b.Input != nil {
		b.Input.Properties = m.in.Apply(b.Input.Properties)
	}
	if m.out != nil && b.Output != nil {
		b.Output.Properties = m.out.Apply(b.Output.Properties)
	}
	if m.req != nil && b.Infra != nil && b.Infra.Buildbucket != nil {
		b.Infra.Buildbucket.RequestedProperties = m.req.Apply(b.Infra.Buildbucket.RequestedProperties)
	}
	if len(m.stepStatuses) > 0 && len(b.Steps) > 0 {
		steps := make([]*pb.Step, 0, len(b.Steps))
		for _, s := range b.Steps {
			if _, ok := m.stepStatuses[s.Status]; ok {
				steps = append(steps, s)
			}
		}
		b.Steps = steps
	}
	return nil
}
