// Copyright 2019 The LUCI Authors.
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

package streamclient

import (
	"time"

	"go.chromium.org/luci/common/clock/clockflag"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
)

type options struct {
	desc       streamproto.Flags
	forProcess bool
}

// Option functions are returned by the With* functions in this package and can
// be passed to the Client.New*Stream methods to alter their contents during
// initial stream setup.
type Option func(*options)

// RenderOptions is a helper function for tests and low-level code.
//
// Returns the cumulative effect of a list of Options.
func RenderOptions(opts ...Option) (flags streamproto.Flags, forProcess bool) {
	acc := options{}
	for _, opt := range opts {
		opt(&acc)
	}
	return acc.desc, acc.forProcess
}

// WithContentType returns an Option to set the content type for a new stream.
//
// The content type can be read by LogDog clients and may be used to indicate
// the type of data in this stream.
//
// By default:
//
//   - Text streams have the type "text/plain"
//   - Binary streams have the type "application/octet-stream"
//   - Datagram streams have the type "application/x-logdog-datagram"
func WithContentType(contentType string) Option {
	return func(o *options) {
		o.desc.ContentType = contentType
	}
}

// WithTimestamp allows you to set the 'starting' timestamp for this stream.
//
// Panics if given a timestamp that ptypes.TimestampProto cannot handle (no real
// timestamps will ever fall into this range).
//
// By default the stream will be marked as starting at `clock.Now(ctx)`.
func WithTimestamp(stamp time.Time) Option {
	return func(o *options) {
		o.desc.Timestamp = clockflag.Time(stamp)
	}
}

// WithTags allows you to add arbitrary tags to this stream (subject to the
// limits in `logdog/common/types.ValidateTag`). This is a convenience version
// of WithTagMap.
//
// You must supply tags in pairs of {key, value}. Otherwise this function
// panics.
//
// By default, streams have no additional tags.
func WithTags(keyValues ...string) Option {
	if len(keyValues)%2 != 0 {
		panic(errors.New("must supply an even number of arguments"))
	}
	m := make(map[string]string, len(keyValues)/2)
	for i := 0; i < len(keyValues); i += 2 {
		m[keyValues[i]] = keyValues[i+1]
	}
	return WithTagMap(m)
}

// WithTagMap allows you to add arbitrary tags to this stream (subject to the
// limits in `logdog/common/types.ValidateTag`).
//
// By default, streams have no additional tags.
func WithTagMap(tags map[string]string) Option {
	return func(o *options) {
		o.desc.Tags = tags
	}
}

// ForProcess is an Option which opens this stream optimized for subprocess IO
// (i.e. to attach to "os/exec".Cmd.Std{out,err}).
//
// Accidentally passing a non-`ForProcess` stream to a subprocess will result in
// an extra pipe, and an extra goroutine with a copy loop in the parent process.
//
// ForProcess is only allowed on Text and Binary streams, not datagram streams.
// This is because the datagram stream is "packet" oriented, but stdout/stderr
// are not. If an application knows enough about the butler protocol to properly
// frame its output, it should just open a butler connection directly, rather
// than emitting framed data on its standard outputs.
//
// Note that when using a stream as a replacement for a process stdout/stderr
// handle, it must be used in the following manner (error checks omitted for
// brevity). The important thing to notice is that stdout must be Close'd after
// Wait (which could be accomplised by a defer). Failure to do this will keep
// the Stream open from the butler's point of view, which will prevent the butler
// from closing down.
//
//	stdout, _ = logdogServ.Client.NewStream(
//	  ..., streamclient.ForProcess())
//	cmd.Stdout = stdout
//
//	cmd.Start()
//	cmd.Wait()
//
//	stdout.Close()
//
// Using this has some caveats, depending on the underlying stream
// implementation:
//
// # Local
//
// Clients created with NewLocal ignore this option.
//
// # Fake
//
// Clients created with NewFake ignore this option.
//
// # Null
//
// The "null" protocol will return an open File pointing to os.DevNull.
//
// # Windows - NamedPipe
//
// This stream will be opened as a synchronous *os.File, and so will be suitable
// for os/exec's specific optimizations for this type (namely: it will be passed
// as a replacement HANDLE for stdout/stderr, and no extra pipe/goroutine will
// be allocated).
//
// Accidentally using a `ForProcess` stream in the current process will result
// in go falling back to its internal IO threadpool to emulate asynchrony.
// Without `ForProcess`, the stream will use Windows-native OVERLAPPED IO
// completion ports, which allows the go process to be much more efficient with
// its thread count.
//
// # Mac and Linux - Unix Domain Sockets
//
// This stream will be dialed as a regular unix socket, but will be converted to
// an *os.File via `"net".UnixConn.File()`, and the original file handle
// ("Conn") closed.
//
// Accidentally using a `ForProcess` stream in the current process isn't
// extremely harmful (because Go has a decent event-based IO system for *NIX
// systems).
func ForProcess() Option {
	return func(o *options) {
		o.forProcess = true
	}
}

// Binary can be used with NewStream to select a binary mode stream.
//
// Using this with NewDatagramStream has no effect.
func Binary() Option {
	return func(o *options) {
		o.desc.Type = streamproto.StreamType(logpb.StreamType_BINARY)
	}
}
