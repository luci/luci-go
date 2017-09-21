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
	"fmt"
	"strings"
	"time"

	"go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/auth/identity"
	"go.chromium.org/luci/common/errors"
)

// CanaryPreference specifies whether a build should use canary of the build
// infrastructure.
type CanaryPreference int

const (
	// NoCanaryPreference specifies that either of prod or canary version of
	// build infrastructure can be used.
	NoCanaryPreference CanaryPreference = iota
	// Prod specifies that the prod version of build infrastructure
	// is preferred.
	Prod
	// Canary specifies that the canary version of build infrastructure
	// is preferred.
	Canary
)

// Build is a buildbucket build.
// It is a more type-safe version of buildbucket.ApiCommonBuildMessage.
type Build struct {

	// fields set at the build creation time

	ID               int64
	CreationTime     time.Time
	CreatedBy        identity.Identity
	Bucket           string
	Builder          string
	Tags             Tags
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
	Properties map[string]interface{}
	Err error // may be populated in a failed build

	// TODO(nodir): add swarmbucket's build run result and swarming task result
}

// ParseBuild parses a build message to Build.
func ParseBuild(raw *buildbucket.ApiCommonBuildMessage) (*Build, error) {
	status, err := ParseStatus(raw)
	if err != nil {
		return nil, err
	}

	createdBy, err := identity.MakeIdentity(raw.CreatedBy)
	if err != nil {
		return nil, err
	}

	tags := ParseTags(raw.Tags)
	builder := tags.Get(TagBuilder)

	b := &Build{
		ID:           raw.Id,
		CreationTime: ParseTimestamp(raw.CreatedTs),
		CreatedBy:    createdBy,
		Bucket:       raw.Bucket,
		Builder:      builder,
		Tags:         tags,

		Status:           status,
		StatusChangeTime: ParseTimestamp(raw.StatusChangedTs),
		URL:              raw.Url,
		UpdateTime:       ParseTimestamp(raw.UpdatedTs),
		Canary:           raw.Canary,

		CompletionTime: ParseTimestamp(raw.CompletedTs),
	}

	switch raw.CanaryPreference {
	case "PROD":
		b.CanaryPreference = Prod
	case "CANARY":
		b.CanaryPreference = Canary
	}

	if err := parseJSON(raw.ParametersJson, &b.Input); err != nil {
		return nil, errors.Annotate(err, "invalid raw.ParametersJson").Err()
	}

	var output struct {
		Properties map[string]interface{}
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
	b.Output.Properties = output.Properties

	return b, nil
}

// PutRequest converts b to a build creation request.
func (b *Build) PutRequest() (*buildbucket.ApiPutRequestMessage, error) {
	msg := &buildbucket.ApiPutRequestMessage{
		Bucket: b.Bucket,
		Tags:   b.Tags.Format(),
	}

	switch b.CanaryPreference {
	case Prod:
		msg.CanaryPreference = "PROD"
	case Canary:
		msg.CanaryPreference = "CANARY"
	case NoCanaryPreference:
		// that's the default
	default:
		return nil, fmt.Errorf("invalid CanaryPreference %v", b.CanaryPreference)
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
	return json.NewDecoder(strings.NewReader(data)).Decode(v)
}

// ParseTimestamp parses a buildbucket timestamp.
func ParseTimestamp(usec int64) time.Time {
	if usec == 0 {
		return time.Time{}
	}
	return time.Unix(usec/1e6, (usec%1e6)*1e3)
}

// FormatTimestamp converts t to a buildbucket timestamp.
func FormatTimestamp(t time.Time) int64 {
	return t.UnixNano() / 1000
}
