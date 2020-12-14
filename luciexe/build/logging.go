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
	"io"

	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
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
	// Log creates a new log stream (by default, line-oriented text) with the
	// given name.
	//
	// To create a binary stream, pass streamclient.Binary() as one of the
	// options.
	//
	// You must close the stream when you're done with it.
	Log(name string, opts ...streamclient.Option) (io.WriteCloser, error)

	// Log creates a new datagram-oriented log stream with the given name.
	//
	// You must close the stream when you're done with it.
	LogDatagram(name string, opts ...streamclient.Option) (streamclient.DatagramStream, error)
}

// LogFromFile is a convenience function which allows you to record a file from
// the filesystem as a log on a Loggable object.
//
// Example:
//
//    LogFromFile(step, "thingy.log", "/path/to/thingy.log")
func LogFromFile(l Loggable, name string, filepath string, opts ...streamclient.Option) error {
	panic("implement me")
}
