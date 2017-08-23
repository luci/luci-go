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

// Package fakelogs implements a fake LogClient for use in tests.
package fakelogs

import (
	"io"

	logs "go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
	logdog_types "go.chromium.org/luci/logdog/common/types"
)

// Stream represents a single logdog stream.
//
// Each invocation of Write() will append a new LogEntry to the stream
// internally. For datagram streams, this means that each Write is a single
// datagram.
//
// Once the Stream is Close()'d it will be marked as complete.
type Stream io.WriteCloser

// Client implements the logs.LogsClient API, and also has some 'reach-around'
// APIs to insert stream data into the backend.
//
// The reach-around APIs are very primitive; they don't simulate butler-side
// constraints (i.e. no prefix secrets, no multi-stream bundling, etc.), however
// from the server-side they should present a sufficient surface to write
// testable server code.
//
// The Open*Stream methods take a singular optional flags object. If provided,
// a copy of this Flags object will be populated with the given logdog path and
// stream type. You can use this to give your streams a content type, tags, etc.
type Client interface {
	logs.LogsClient

	OpenTextStream(prefix, path logdog_types.StreamName, flags ...*streamproto.Flags) (Stream, error)
	OpenDatagramStream(prefix, path logdog_types.StreamName, flags ...*streamproto.Flags) (Stream, error)
	OpenBinaryStream(prefix, path logdog_types.StreamName, flags ...*streamproto.Flags) (Stream, error)
}

// NewClient generates a new fake Client which can be used as a logs.LogsClient,
// and can also have its underlying stream data manipulated by the test.
//
// Functions taking context.Context will ignore it (i.e. they don't expect
// anything in the context).
func NewClient() Client {
	panic("not implemented")
}
