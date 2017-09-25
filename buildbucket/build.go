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

package buildbucket

import (
	"encoding/json"
	"strings"
	"time"

	"go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/auth/identity"
	"go.chromium.org/luci/common/data/strtag"
	"go.chromium.org/luci/common/errors"
)

// TagBuilder is the key of builder name tag.
const TagBuilder = "builder"

// CanaryPreference specifies whether a build should use canary of the build
// infrastructure.
type CanaryPreference string

const (
	// NoCanaryPreference specifies that either of prod or canary version of
	// build infrastructure can be used.
	NoCanaryPreference CanaryPreference = "AUTO"
	// Prod specifies that the prod version of build infrastructure
	// is preferred.
	Prod = "PROD"
	// Canary specifies that the canary version of build infrastructure
	// is preferred.
	Canary = "CANARY"
)

// Build is a buildbucket build.
// It is a more type-safe version of buildbucket.ApiCommonBuildMessage.
type Build struct {

	// fields set at the build creation time

	ID           int64
	CreationTime time.Time
	CreatedBy    identity.Identity
	Bucket       string
	Builder      string
	// BuildSets is parsed "buildset" tag values.
	//
	// If a buildset is present in tags, but could not recognized
	// it won't be included here.
	BuildSets        []BuildSet
	Tags             strtag.Tags
	Input            Input
	CanaryPreference CanaryPreference

	// fields that can change during build lifetime

	Status           Status
	StatusChangeTime time.Time
	URL              string
	UpdateTime       time.Time
	Canary           bool

	// fields set on build completion

	CompletionTime time.Time
	Output         Output
}

// Input is the input to the builder.
type Input struct {
	Properties map[string]interface{}

	// TODO(nodir): add support for changes
	// https://chromium.googlesource.com/chromium/tools/build/+/master/scripts/master/buildbucket/README.md#build-parameters
}

// Output is build output.
type Output struct {
	// No properties here. Swarmbucket does not expose properties because
	// luci-eng@ doesn't want to expose them, because they are bad API.
	// TODO(nodir, iannucci): update this when we establish an API for recipes
	// to export data.

	Err error // may be populated in a failed build

	// TODO(nodir): add swarmbucket's build run result and swarming task result
}

// ParseBuild parses a build message to Build.
// Numeric values in JSON-formatted fields, e.g. property values, are parsed as
// json.Number.
func ParseBuild(raw *buildbucket.ApiCommonBuildMessage) (*Build, error) {
	status, err := ParseStatus(raw)
	if err != nil {
		return nil, err
	}

	createdBy, err := identity.MakeIdentity(raw.CreatedBy)
	if err != nil {
		return nil, err
	}

	tags := strtag.Parse(raw.Tags)
	builder := tags.Get(TagBuilder)

	b := &Build{
		ID:               raw.Id,
		CreationTime:     ParseTimestamp(raw.CreatedTs),
		CreatedBy:        createdBy,
		Bucket:           raw.Bucket,
		Builder:          builder,
		Tags:             tags,
		CanaryPreference: CanaryPreference(raw.CanaryPreference),

		Status:           status,
		StatusChangeTime: ParseTimestamp(raw.StatusChangedTs),
		URL:              raw.Url,
		UpdateTime:       ParseTimestamp(raw.UpdatedTs),
		Canary:           raw.Canary,

		CompletionTime: ParseTimestamp(raw.CompletedTs),
	}
	for _, bs := range tags[TagBuildSet] {
		if parsed := ParseBuildSet(bs); parsed != nil {
			b.BuildSets = append(b.BuildSets, parsed)
		}
	}

	if err := parseJSON(raw.ParametersJson, &b.Input); err != nil {
		return nil, errors.Annotate(err, "invalid raw.ParametersJson").Err()
	}

	var output struct {
		Error struct {
			Message string
		}
	}
	if err := parseJSON(raw.ResultDetailsJson, &output); err != nil {
		return nil, errors.Annotate(err, "invalid raw.ResultDetailsJson").Err()
	}
	if output.Error.Message != "" {
		b.Output.Err = errors.New(output.Error.Message)
	}

	return b, nil
}

// PutRequest converts b to a build creation request.
// If a buildset is present in both b.BuildSets and b.Tags, it is deduped.
// Returned value has zero ClientOperationId.
// Returns an error if properties could not be marshaled to JSON.
func (b *Build) PutRequest() (*buildbucket.ApiPutRequestMessage, error) {
	tags := b.Tags.Copy()
	tags.Del(TagBuilder) // buildbucket adds it automatically
	for _, bs := range b.BuildSets {
		s := bs.String()
		if !tags.Contains(TagBuildSet, s) {
			tags.Add(TagBuildSet, s)
		}
	}

	msg := &buildbucket.ApiPutRequestMessage{
		Bucket:           b.Bucket,
		Tags:             tags.Format(),
		CanaryPreference: string(b.CanaryPreference),
	}

	parameters := map[string]interface{}{
		"builder_name": b.Builder,
		"properties":   b.Input.Properties,
	}
	if data, err := json.Marshal(parameters); err != nil {
		return nil, err
	} else {
		msg.ParametersJson = string(data)
	}

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

// ParseTimestamp parses a buildbucket timestamp.
// Year 2250+ is not supported.
func ParseTimestamp(usec int64) time.Time {
	if usec == 0 {
		return time.Time{}
	}
	return time.Unix(0, usec*1000).UTC()
}

// FormatTimestamp converts t to a buildbucket timestamp.
func FormatTimestamp(t time.Time) int64 {
	return t.UnixNano() / 1000
}
