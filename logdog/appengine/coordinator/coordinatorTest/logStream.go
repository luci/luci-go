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

package coordinatorTest

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	ds "go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth/realms"

	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/logdog/common/types"
)

// TestSecret returns a testing types.StreamPrefix.
func TestSecret() types.PrefixSecret {
	return types.PrefixSecret(bytes.Repeat([]byte{0x6F}, types.PrefixSecretLength))
}

// TestStream returns a testing stream.
type TestStream struct {
	// Project is the project name for this stream.
	Project string
	// Realm is the realm name within the project for this stream.
	Realm string
	// Path is the path of this stream.
	Path types.StreamPath

	// Desc is the log stream descriptor.
	Desc *logpb.LogStreamDescriptor

	// Prefix is the Coordinator LogPrefix entity.
	Prefix *coordinator.LogPrefix
	// Prefix is the Coordinator LogStreamState entity.
	State *coordinator.LogStreamState
	// Prefix is the Coordinator LogStream entity.
	Stream *coordinator.LogStream
}

// MakeStream builds a new TestStream with the supplied parameters.
func MakeStream(c context.Context, project, realm string, path types.StreamPath) *TestStream {
	prefix, name := path.Split()

	now := clock.Now(c).UTC()
	secret := TestSecret()

	ts := TestStream{
		Project: project,
		Realm:   realm,
		Prefix: &coordinator.LogPrefix{
			ID:         "", // Filled in by Reload.
			Created:    ds.RoundTime(now),
			Prefix:     "", // Filled in by Reload.
			Realm:      "", // Filled in by Reload.
			Source:     []string{"test suite"},
			Expiration: ds.RoundTime(now.Add(24 * time.Hour)),
			Secret:     secret,
		},
		Desc: &logpb.LogStreamDescriptor{
			Prefix:      string(prefix),
			Name:        string(name),
			StreamType:  logpb.StreamType_TEXT,
			ContentType: "application/text",
			Timestamp:   timestamppb.New(now),
		},
		State: &coordinator.LogStreamState{
			Parent:        nil, // Filled in by Reload.
			Created:       ds.RoundTime(now),
			Updated:       time.Time{}, // Filled in by Reload.
			ExpireAt:      ds.RoundTime(now.Add(coordinator.LogStreamStateExpiry)),
			Secret:        secret,
			TerminalIndex: -1,
		},
		Stream: &coordinator.LogStream{
			ID:           "", // Filled in by Reload.
			ProtoVersion: logpb.Version,
			Created:      ds.RoundTime(now),
			ExpireAt:     ds.RoundTime(now.Add(coordinator.LogStreamExpiry)),
			// Descriptor-derived fields filled in by Reload.
		},
	}
	ts.Reload(c)
	return &ts
}

// Reload loads derived fields from their base fields.
func (ts *TestStream) Reload(c context.Context) {
	ts.Path = ts.Desc.Path()

	// LogPrefix
	ts.Prefix.Prefix = ts.Desc.Prefix
	ts.Prefix.ID = coordinator.LogPrefixID(types.StreamName(ts.Prefix.Prefix))
	if ts.Realm != "" {
		ts.Prefix.Realm = realms.Join(ts.Project, ts.Realm)
	} else {
		ts.Prefix.Realm = ""
	}

	// LogStream
	ts.Stream.ID = coordinator.LogStreamID(ts.Path)
	if err := ts.Stream.LoadDescriptor(ts.Desc); err != nil {
		panic(err)
	}

	// LogStreamState
	ts.State.Updated = ds.RoundTime(clock.Now(c)).UTC()
	ts.WithProjectNamespace(c, func(c context.Context) {
		ts.State.Parent = ds.KeyForObj(c, ts.Stream)
	})
}

// DescBytes returns the marshalled descriptor bytes.
func (ts *TestStream) DescBytes() []byte {
	v, err := proto.Marshal(ts.Desc)
	if err != nil {
		panic(err)
	}
	return v
}

// Put adds all of the entities for this TestStream to the datastore.
func (ts *TestStream) Put(c context.Context) (err error) {
	ts.WithProjectNamespace(c, func(c context.Context) {
		err = ds.Put(c, ts.Prefix, ts.State, ts.Stream)
	})
	return
}

// Get reloads all of the entities for this TestStream.
func (ts *TestStream) Get(c context.Context) (err error) {
	ts.WithProjectNamespace(c, func(c context.Context) {
		err = ds.Get(c, ts.Prefix, ts.State, ts.Stream)
	})
	return
}

// LogEntry generates a generic testing log entry for this stream with the
// specific log stream index.
func (ts *TestStream) LogEntry(c context.Context, i int) *logpb.LogEntry {
	le := logpb.LogEntry{
		TimeOffset:  durationpb.New(clock.Now(c).Sub(ts.Stream.Created)),
		StreamIndex: uint64(i),
	}

	message := fmt.Sprintf("log entry #%d", i)
	switch ts.Desc.StreamType {
	case logpb.StreamType_TEXT:
		le.Content = &logpb.LogEntry_Text{
			Text: &logpb.Text{
				Lines: []*logpb.Text_Line{
					{
						Value:     []byte(message),
						Delimiter: "\n",
					},
				},
			},
		}

	case logpb.StreamType_BINARY:
		le.Content = &logpb.LogEntry_Binary{
			Binary: &logpb.Binary{
				Data: []byte(message),
			},
		}

	case logpb.StreamType_DATAGRAM:
		le.Content = &logpb.LogEntry_Datagram{
			Datagram: &logpb.Datagram{
				Data: []byte(message),
			},
		}
	}
	return &le
}

// WithProjectNamespace runs f in proj's namespace, bypassing authentication
// checks.
func (ts *TestStream) WithProjectNamespace(c context.Context, f func(context.Context)) {
	WithProjectNamespace(c, ts.Project, f)
}
