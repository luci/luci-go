// Copyright 2020 The LUCI Authors.
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

package build

import (
	"context"
	"io"
)

// Loggable is the common interface for build entities which have log data
// associated with them.
//
// Implemented by State and StepState.
//
// Logs all have a name which is an arbitrary bit of text to identify the log to
// human users (it will appear as the link on the build UI page). In particular
// it does NOT need to conform to the LogDog stream name alphabet.
//
// The log name "log" is reserved, and will automatically capture all logging
// outputs generated with the "go.chromium.org/luci/common/logging" API.
type Loggable interface {
	// Log creates a new line-oriented text log stream with the given name.
	//
	// You must close the stream when you're done with it.
	Log(ctx context.Context, name string) (io.WriteCloser, error)

	// LogBinary creates a new binary log stream with the given name.
	//
	// You must close the stream when you're done with it.
	LogBinary(ctx context.Context, name string) (io.WriteCloser, error)

	// LogDatagram creates a new datagram log stream with the given name.
	// Each call to WriteDatagram will produce a single datagram message in the
	// stream.
	//
	// You must close the stream when you're done with it.
	LogDatagram(ctx context.Context, name string) (DatagramWriter, error)

	// LogFile is a helper method which copies the contents of the file at
	// `filepath` to the line-oriented log named `name`.
	LogFile(ctx context.Context, name, filepath string) error

	// LogBinaryFile is a helper method which copies the contents of the file at
	// `filepath` to the binary log named `name`.
	LogBinaryFile(ctx context.Context, name, filepath string) error
}

// DatagramWriter is a minimal interface for writing datagram-based log streams.
type DatagramWriter interface {
	io.Closer

	// WriteDatagram writes `dg` as a single datagram to the underlying log
	// stream.
	WriteDatagram(dg []byte) error
}
