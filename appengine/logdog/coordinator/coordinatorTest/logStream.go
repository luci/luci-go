// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinatorTest

import (
	"bytes"
	"fmt"

	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/common/proto/google"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
	"golang.org/x/net/context"
)

// TestLogStreamDescriptor generates a stock testing LogStreamDescriptor
// protobuf.
func TestLogStreamDescriptor(c context.Context, name string) *logpb.LogStreamDescriptor {
	return &logpb.LogStreamDescriptor{
		Prefix:      "testing",
		Name:        name,
		StreamType:  logpb.LogStreamDescriptor_TEXT,
		ContentType: "application/text",
		Timestamp:   google.NewTimestamp(clock.Now(c)),
		Tags: []*logpb.LogStreamDescriptor_Tag{
			{"foo", "bar"},
			{"baz", "qux"},
			{"quux", ""},
		},
	}
}

// TestLogStream generates a stock testing LogStream from a LogStreamDescriptor.
func TestLogStream(c context.Context, desc *logpb.LogStreamDescriptor) (*coordinator.LogStream, error) {
	ls, err := coordinator.NewLogStream(string(desc.Path()))
	if err != nil {
		return nil, err
	}
	if err := ls.LoadDescriptor(desc); err != nil {
		return nil, err
	}

	ls.ProtoVersion = logpb.Version
	ls.Created = coordinator.NormalizeTime(clock.Now(c).UTC())
	ls.Updated = coordinator.NormalizeTime(clock.Now(c).UTC())
	ls.Secret = bytes.Repeat([]byte{0x6F}, types.StreamSecretLength)
	ls.TerminalIndex = -1
	return ls, nil
}

// TestLogEntry generates a standard testing text logpb.LogEntry.
func TestLogEntry(c context.Context, ls *coordinator.LogStream, i int) *logpb.LogEntry {
	return &logpb.LogEntry{
		TimeOffset:  google.NewDuration(clock.Now(c).Sub(ls.Created)),
		StreamIndex: uint64(i),

		Content: &logpb.LogEntry_Text{
			&logpb.Text{
				Lines: []*logpb.Text_Line{
					{
						Value:     fmt.Sprintf("log entry #%d", i),
						Delimiter: "\n",
					},
				},
			},
		},
	}
}
