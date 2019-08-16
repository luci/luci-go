// Copyright 2015 The LUCI Authors.
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

package streamproto

import (
	"time"

	"go.chromium.org/luci/common/clock/clockflag"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/types"
)

// Flags is a flag- and JSON-compatible version of logpb.LogStreamDescriptor.
// It is used for stream negotiation protocol and command-line interfaces.
//
// TODO(iannucci) - Change client->butler protocol to just use jsonpb encoding
// of LogStreamDescriptor.
type Flags struct {
	Name        StreamNameFlag `json:"name,omitempty"`
	ContentType string         `json:"contentType,omitempty"`
	Type        StreamType     `json:"type,omitempty"`
	Timestamp   clockflag.Time `json:"timestamp,omitempty"`
	Tags        TagMap         `json:"tags,omitempty"`
}

// Descriptor converts the Flags to a LogStreamDescriptor.
func (f *Flags) Descriptor() *logpb.LogStreamDescriptor {
	contentType := types.ContentType(f.ContentType)
	if contentType == "" {
		contentType = f.Type.DefaultContentType()
	}

	return &logpb.LogStreamDescriptor{
		Name:        string(f.Name),
		ContentType: string(contentType),
		StreamType:  logpb.StreamType(f.Type),
		Timestamp:   google.NewTimestamp(time.Time(f.Timestamp)),
		Tags:        f.Tags,
	}
}
