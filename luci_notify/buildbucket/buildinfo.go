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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/context"

	bbapi "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
)

// BuildInfo is a wrapper struct that provides easier access to
// the buildbucket API's build message.
type BuildInfo struct {
	// Build contains the raw information sent by buildbucket.
	Build bbapi.ApiCommonBuildMessage `json:"build"`

	// Hostname is the hostname of the buildbucket instance that sent the message.
	Hostname string `json:"hostname"`

	// BuilderName is the name of the builder, stored here because
	// extracting it from Build is non-trivial.
	BuilderName string
}

// extractBuildName gets the builder name from the message.
//
// The builder name for a given build message is stored in two
// locations: in parameters_json and as a tag ("builder:<name>").
// This function extracts it from parameters_json.
func extractBuilderName(build *bbapi.ApiCommonBuildMessage) (string, error) {
	type parameters struct {
		BuilderName string `json:"builder_name"`
	}
	// We only care about the "builder_name" key from the parameter.
	p := parameters{}
	err := json.Unmarshal([]byte(build.ParametersJson), &p)
	if err != nil {
		err = errors.Annotate(
			err, "could not unmarshal build parameters %s", build.ParametersJson).Err()
		// Permanent error, since this is probably a type of build we do not recognize.
		return "", err
	}
	return p.BuilderName, nil
}

// GetBuilderID constructs a canonical builder ID from the build's bucket and
// the build's builder's name.
func (b *BuildInfo) GetBuilderID() string {
	return fmt.Sprintf("buildbucket/%s/%s", b.Build.Bucket, b.BuilderName)
}

// GetCreatedTime returns a time.Time for the time the build was created.
func (b *BuildInfo) GetCreatedTime() time.Time {
	return time.Unix(b.Build.CreatedTs/1000000, 0)
}

// GetCompletedTime returns a time.Time for the time the build was completed.
func (b *BuildInfo) GetCompletedTime() time.Time {
	return time.Unix(b.Build.CompletedTs/1000000, 0)
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

// These structs reflect (in json) the structure of the PubSub message sent.

type pubSubMessage struct {
	Attributes map[string]string `json:"attributes"`
	Data       string            `json:"data"`
	MessageID  string            `json:"message_id"`
}

type pubSubSubscription struct {
	Message      pubSubMessage `json:"message"`
	Subscription string        `json:"subscription"`
}

// ExtractBuildInfo constructs a BuildInfo from the PubSub http request.
func ExtractBuildInfo(c context.Context, r *http.Request) (*BuildInfo, error) {
	var message BuildInfo
	// This struct is just convenient for unwrapping the json message
	// send by pubsub.
	msg := pubSubSubscription{}
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&msg); err != nil {
		// This might be a transient error, e.g. when the json format changes
		// and luci-notify isn't updated yet.
		err = errors.Annotate(err, "could not decode message:\n%s", r.Body).Err()
		return nil, transient.Tag.Apply(err)
	}
	// The actual build information is encoded in base64.
	data, err := base64.StdEncoding.DecodeString(msg.Message.Data)
	if err != nil {
		return nil, errors.Annotate(err, "could not parse pubsub message string").Err()
	}
	if err := json.Unmarshal(data, &message); err != nil {
		return nil, errors.Annotate(err, "could not parse pubsub message data").Err()
	}
	name, err := extractBuilderName(&message.Build)
	if err != nil {
		return nil, errors.Annotate(err, "could not find a builder name").Err()
	}
	// "Cache" the builder name so we don't have to extract it every time.
	message.BuilderName = name
	return &message, nil
}
