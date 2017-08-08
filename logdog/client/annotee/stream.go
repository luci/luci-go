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

package annotee

import (
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/clockflag"
	"go.chromium.org/luci/common/proto/milo"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
	"go.chromium.org/luci/logdog/common/types"

	"golang.org/x/net/context"
)

var (
	textStreamArchetype = streamproto.Flags{
		ContentType: string(types.ContentTypeText),
		Type:        streamproto.StreamType(logpb.StreamType_TEXT),
	}

	metadataStreamArchetype = streamproto.Flags{
		ContentType: string(milo.ContentTypeAnnotations),
		Type:        streamproto.StreamType(logpb.StreamType_DATAGRAM),
	}
)

// TextStreamFlags returns the streamproto.Flags for a text stream using
// Annotee's text stream archetype.
func TextStreamFlags(ctx context.Context, name types.StreamName) streamproto.Flags {
	return streamFlagsFromArchetype(ctx, name, &textStreamArchetype)
}

func streamFlagsFromArchetype(ctx context.Context, name types.StreamName, archetype *streamproto.Flags) streamproto.Flags {
	// Clone the properties archetype and customize.
	f := *archetype
	f.Timestamp = clockflag.Time(clock.Now(ctx))
	f.Name = streamproto.StreamNameFlag(name)
	return f
}
