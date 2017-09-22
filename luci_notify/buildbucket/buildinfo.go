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
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/context"

	bbapi "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/errors"
)

// BuildParameters reflects the field ParametersJson in ApiCommonBuildMessage in
// its decoded form.
type BuildParameters struct {
	// BuilderName is the name of the builder for a particular build.
	BuilderName string `json:"builder_name"`

	// TODO(mknyszek): Support input properties here (e.g. revision, branch).
}

// BuildInfo is a wrapper struct that provides easier access to
// the buildbucket API's build message.
type BuildInfo struct {
	// Build contains the raw information sent by buildbucket.
	Build bbapi.ApiCommonBuildMessage `json:"build"`

	// Hostname is the hostname of the buildbucket instance that sent the message.
	Hostname string `json:"hostname"`

	// Parameters is the decoded form of Build.ParametersJson, cached here
	// for convenience.
	Parameters BuildParameters
}

// extractBuildParameters gets the build parameters from build.
//
// This function decodes ParametersJson in ApiCommonBuildMessage into
// BuildParameters at a specified location. Primarily intended to be used
// to populate BuildInfo.Parameters.
func extractBuildParameters(params *BuildParameters, build *bbapi.ApiCommonBuildMessage) error {
	// We only care about the "builder_name" key from the parameter.
	err := json.Unmarshal([]byte(build.ParametersJson), params)
	if err != nil {
		err = errors.Annotate(
			err, "could not unmarshal build parameters %s", build.ParametersJson).Err()
		// Permanent error, since this is probably a type of build we do not recognize.
		return err
	}
	return nil
}

// parseTimestamp parses a buildbucket timestamp.
func parseTimestamp(usec int64) time.Time {
	if usec == 0 {
		return time.Time{}
	}
	return time.Unix(usec/1e6, (usec%1e6)*1e3)
}

// BuilderID constructs a canonical builder ID from the build's bucket and
// the build's builder's name.
func (b *BuildInfo) BuilderID() string {
	return fmt.Sprintf("buildbucket/%s/%s", b.Build.Bucket, b.Parameters.BuilderName)
}

// CreatedTime returns a time.Time for the time the build was created.
func (b *BuildInfo) CreatedTime() time.Time {
	return parseTimestamp(b.Build.CreatedTs)
}

// CompletedTime returns a time.Time for the time the build was completed.
func (b *BuildInfo) CompletedTime() time.Time {
	return parseTimestamp(b.Build.CompletedTs)
}

// IsLUCI returns true if the build was created and managed by LUCI.
func (b *BuildInfo) IsLUCI() bool {
	// All luci buckets are assumed to be prefixed with luci.
	return strings.HasPrefix(b.Build.Bucket, "luci.")
}

// IsAllowed returns true if luci-notify is allowed to handle this build.
func (b *BuildInfo) IsAllowed() bool {
	// TODO(mknyszek): Do a real ACL check here on whether the service should
	// be allowed to process the build. This is a conservative solution for now
	// which ensures that the build is public.
	for _, tag := range b.Build.Tags {
		if tag == "swarming_tag:allow_milo:1" {
			return true
		}
	}
	return false
}

// ExtractBuildInfo constructs a BuildInfo from the PubSub http request.
func ExtractBuildInfo(c context.Context, r *http.Request) (*BuildInfo, error) {
	// This struct is just convenient for unwrapping the json message
	// send by pubsub.
	var msg struct {
		Message struct {
			Data []byte
		}
	}
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		return nil, errors.Annotate(err, "could not decode message").Err()
	}

	var message BuildInfo
	if err := json.Unmarshal(msg.Message.Data, &message); err != nil {
		return nil, errors.Annotate(err, "could not parse pubsub message data").Err()
	}
	if err := extractBuildParameters(&message.Parameters, &message.Build); err != nil {
		return nil, errors.Annotate(err, "could not decode build parameters").Err()
	}
	return &message, nil
}
