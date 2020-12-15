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
	"golang.org/x/time/rate"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
)

// StartOption is an object which can be passed to the Start function, and
// modifies the behavior of the luciexe/build library.
//
// StartOptions are exclusively constructed from the Opt* functions in this
// package.
//
// StartOptions are all unique per Start (i.e. you can only pass one of a kind
// per option to Start).
type StartOption func(*State)

// OptLogsink allows you to associate a streamclient with the started build.
//
// See `streamclient.New` and `streamclient.NewFake` for how to create a client
// suitable to your needs (note that this includes a local filesystem option).
//
// If a logsink is configured, it will be used as the output destination for the
// go.chromium.org/luci/common/logging library, and will recieve all data
// written via the Loggable interface.
//
// If no logsink is configured, the go.chromium.org/luci/common/logging library
// will be unaffected, and data written to the Loggable interface will go to
// an ioutil.NopWriteCloser.
func OptLogsink(*streamclient.Client) StartOption {
	panic("not implemented")
}

// OptSend allows you to get a callback when the state of the underlying Build
// changes.
//
// This callback will be called at most as frequently as `rate` allows, up to
// once per Build change, and is called with a copy of Build. Only one
// outstanding invocation of this callback can occur at once.
//
// If new updates come in while this callback is blocking, they will apply
// silently in the background, and as soon as the callback returns (and rate
// allows), it will be invoked again with the current Build state.
func OptSend(rate.Limit, func(*bbpb.Build)) StartOption {
	panic("not implemented")
}

// OptSuppressExit will supress the call to os.Exit from Start in the event that
// a terminal command-line flag was passed (like `--help`).
//
// Instead, Start will return a nil `*State` and an unmodified context.
func OptSuppressExit() StartOption {
	panic("not implemented")
}
