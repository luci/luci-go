// Copyright 2017 The LUCI Authors.
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

package deprecated

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/buildbucket/proto"
	v1 "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
)

// Build is a buildbucket build.
// It is a more type-safe version of buildbucket.LegacyApiCommonBuildMessage.
//
// DEPRECATED: use BuildToV2.
type Build struct {
	// fields set at the build creation time, immutable.

	ID           int64
	CreationTime time.Time
	CreatedBy    identity.Identity
	Project      string
	Bucket       string
	Builder      string
	// Number identifies the build within the builder.
	// Build numbers are monotonically increasing, mostly contiguous.
	//
	// The type is *int to prevent accidental confusion
	// of valid build number 0 with absence of the number (zero value).
	Number *int
	Tags   strpair.Map
	Input  Input

	// fields that can change during build lifetime

	Status           pb.Status
	StatusChangeTime time.Time
	URL              string
	StartTime        time.Time
	UpdateTime       time.Time
	Canary           bool
	Experimental     bool

	// fields set on build completion

	CompletionTime time.Time
	Output         Output
}

// Address returns an alternative identifier of the build.
// If b has a number, the address is "<bucket>/<builder>/<number>".
// Otherwise it is "<id>".
//
// See also "go.chromium.org/luci/common/api/buildbucket/v1".FormatBuildAddress.
func (b *Build) Address() string {
	num := 0
	if b.Number != nil {
		num = *b.Number
	}
	return v1.FormatBuildAddress(b.ID, b.Bucket, b.Builder, num)
}

// RunDuration returns duration between build start and completion.
func (b *Build) RunDuration() (duration time.Duration, ok bool) {
	if b.StartTime.IsZero() || b.CompletionTime.IsZero() {
		return 0, false
	}
	return b.CompletionTime.Sub(b.StartTime), true
}

// SchedulingDuration returns duration between build creation and start.
func (b *Build) SchedulingDuration() (duration time.Duration, ok bool) {
	if b.CreationTime.IsZero() || b.StartTime.IsZero() {
		return 0, false
	}
	return b.StartTime.Sub(b.CreationTime), true
}

// Properties is data provided by users, opaque to LUCI services.
// The value must be JSON marshalable/unmarshalable into/out from a
// JSON object.
//
// When using an unmarshaling function, such as (*Build).ParseMessage,
// if the user knows the properties they need, they may set the
// value to a json-compatible struct. The unmarshaling function will try
// to unmarshal the properties into the struct. Otherwise, the unmarshaling
// function will use a generic type, e.g. map[string]interface{}.
//
// Example:
//
//   var props struct {
//     A string
//   }
//   var build buildbucket.Build
//   build.Input.Properties = &props
//   if err := build.ParseMessage(msg); err != nil {
//     return err
//   }
//   println(props.A)
type Properties interface{}

// Input is the input to the builder.
type Input struct {
	// Properties is opaque data passed to the build.
	// For recipe-based builds, this is build properties.
	Properties Properties
}

// Output is build output.
type Output struct {
	Properties Properties

	// TODO(nodir, iannucci): replace type "error" with a new type that
	// represents a stack of errors emitted by different layers of the system,
	// where each error has
	// - domain string, e.g. "kitchen"
	// - reason string, e.g. kitchen-specific error code
	// - message string: human readable error
	// - meta: a proto.Struct with random data provided by the layer
	// The new type must implement error so that the change is
	// backward compatible.
	Err error // populated in builds with status StatusError
}

// ParseMessage parses a build message to Build.
//
// Numeric values in JSON-formatted fields, e.g. property values, are parsed as
// json.Number.
//
// If an error is returned, the state of b is undefined.
//
// DEPRECATED: use BuildToV2.
func (b *Build) ParseMessage(msg *v1.LegacyApiCommonBuildMessage) error {
	status, err := StatusToV2(msg)
	if err != nil {
		return err
	}

	var createdBy identity.Identity
	if msg.CreatedBy != "" {
		createdBy, err = identity.MakeIdentity(msg.CreatedBy)
		if err != nil {
			return err
		}
	}

	tags := strpair.ParseMap(msg.Tags)
	builder := tags.Get(v1.TagBuilder)

	address := tags.Get(v1.TagBuildAddress)
	var number *int
	if address == "" {
		address = strconv.FormatInt(msg.Id, 10)
	} else {
		parts := strings.Split(address, "/")
		if len(parts) != 3 {
			return fmt.Errorf("invalid build_address %q: expected exactly 2 slashes", address)
		}
		if msg.Bucket == "" {
			// this is a partial response message
			msg.Bucket = parts[0]
		} else if msg.Bucket != parts[0] {
			return fmt.Errorf("invalid build_address %q: expected first component to be %q", address, msg.Bucket)
		}
		if builder == "" {
			return fmt.Errorf("build_address tag is present, but builder tag is not")
		} else if parts[1] != builder {
			return fmt.Errorf("invalid build_address %q: expected second component to be %q", address, builder)
		}
		num, err := strconv.Atoi(parts[2])
		if err != nil {
			return fmt.Errorf("invalid build_address %q: expected third component to be a valid int32", address)
		}
		number = &num
	}

	input := struct{ Properties interface{} }{b.Input.Properties}
	if err := parseJSON(msg.ParametersJson, &input); err != nil {
		return errors.Annotate(err, "invalid msg.ParametersJson").Err()
	}

	output := struct {
		Properties interface{}
		Error      struct {
			Message string
		}
	}{Properties: b.Output.Properties}
	if err := parseJSON(msg.ResultDetailsJson, &output); err != nil {
		return errors.Annotate(err, "invalid msg.ResultDetailsJson").Err()
	}
	var outErr error
	if output.Error.Message != "" {
		outErr = errors.New(output.Error.Message)
	}

	project := msg.Project
	if project == "" {
		// old builds do not have project attribute.
		project = v1.ProjectFromBucket(msg.Bucket)
	}

	*b = Build{
		ID:           msg.Id,
		CreationTime: v1.ParseTimestamp(msg.CreatedTs),
		CreatedBy:    createdBy,
		Project:      msg.Project,
		Bucket:       msg.Bucket,
		Builder:      builder,
		Number:       number,
		Tags:         tags,
		Input: Input{
			Properties: input.Properties,
		},

		Status:           status,
		StatusChangeTime: v1.ParseTimestamp(msg.StatusChangedTs),
		URL:              msg.Url,
		StartTime:        v1.ParseTimestamp(msg.StartedTs),
		UpdateTime:       v1.ParseTimestamp(msg.UpdatedTs),
		Canary:           msg.Canary,
		Experimental:     msg.Experimental,

		CompletionTime: v1.ParseTimestamp(msg.CompletedTs),
		Output: Output{
			Properties: output.Properties,
			Err:        outErr,
		},
	}
	return nil
}

// PutRequest converts b to a build creation request.
//
// If a buildset is present in both b.BuildSets and b.Map, it is deduped.
// Returned value has zero ClientOperationId.
// Returns an error if properties could not be marshaled to JSON.
func (b *Build) PutRequest() (*v1.LegacyApiPutRequestMessage, error) {
	tags := b.Tags.Copy()
	tags.Del(v1.TagBuilder) // buildbucket adds it automatically
	for _, bs := range b.Tags[v1.TagBuildSet] {
		if !tags.Contains(v1.TagBuildSet, bs) {
			tags.Add(v1.TagBuildSet, bs)
		}
	}

	msg := &v1.LegacyApiPutRequestMessage{
		Bucket: b.Bucket,
		Tags:   tags.Format(),
	}

	parameters := map[string]interface{}{
		"builder_name": b.Builder,
		"properties":   b.Input.Properties,
		// keep this synced with marshaling error annotation
	}
	data, err := json.Marshal(parameters)
	if err != nil {
		// realistically, only properties may cause this.
		return nil, errors.Annotate(err, "marshaling properties").Err()
	}
	msg.ParametersJson = string(data)
	return msg, nil
}

func parseJSON(data string, v interface{}) error {
	if data == "" {
		return nil
	}
	dec := json.NewDecoder(strings.NewReader(data))
	dec.UseNumber()
	return dec.Decode(v)
}

// GetByAddress fetches a build by its address.
// Returns (nil, nil) if build is not found.
func GetByAddress(c context.Context, client *v1.Service, address string) (*Build, error) {
	msg, err := v1.GetByAddress(c, client, address)
	if err != nil {
		return nil, err
	}

	var build Build
	err = build.ParseMessage(msg)
	return &build, err
}
