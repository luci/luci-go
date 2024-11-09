// Copyright 2018 The LUCI Authors.
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

package butler

import (
	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/types"
)

// StreamRegistrationCallback is a callback to invoke when a new stream is
// registered with this butler.
//
// Expects passed *logpb.LogStreamDescriptor reference to be safe to keep, and
// should treat it as read-only.
//
// See streamConfig.callback.
type StreamRegistrationCallback func(*logpb.LogStreamDescriptor) StreamChunkCallback

// StreamChunkCallback is a callback to invoke on a complete LogEntry.
//
// Called once with nil when this stream has come to an end.
//
// See streamConfig.callback.
type StreamChunkCallback func(*logpb.LogEntry)

// AddStreamRegistrationCallback adds a new callback to this Butler.
//
// The callback is called on new streams and returns a chunk callback to attach
// to the stream or nil if you don't want to monitor the stream.
//
// If multiple callbacks are all interested in the same stream, the first one
// wins.
//
// # Wrapping
//
// If `wrap` is true, the callback is wrapped internally to buffer LogEntries
// until they're complete.
//
// In wrapped callbacks for text and datagram streams, LogEntry .TimeOffset,
// and .PrefixIndex will be 0. .StreamIndex and .Sequence WILL NOT correspond
// to the values that the logdog service sees. They will, however, be
// internally consistent within the stream.
//
// Wrapped datagram streams never send a partial datagram; If the logdog
// server or stream is shut down while we have a partial datagram buffered,
// the partially buffered datagram will not be observed by the buffered
// callback.
//
// Wrapping a binary stream is a noop (i.e. your callback will see the exact
// same values wrapped and unwrapped).
//
// When the stream ends (either due to EOF from the user, or when the butler
// is stopped), your callback will be invoked exactly once with `nil`.
func (b *Butler) AddStreamRegistrationCallback(cb StreamRegistrationCallback, wrap bool) {
	b.streamRegistrationCallbacksMu.Lock()
	defer b.streamRegistrationCallbacksMu.Unlock()
	b.streamRegistrationCallbacks = append(b.streamRegistrationCallbacks,
		registeredCallback{cb, wrap})
}

func (b *Butler) maybeAddStreamCallback(d *logpb.LogStreamDescriptor) {
	b.streamRegistrationCallbacksMu.RLock()
	defer b.streamRegistrationCallbacksMu.RUnlock()
	for _, cb := range b.streamRegistrationCallbacks {
		if ret := cb.cb(proto.Clone(d).(*logpb.LogStreamDescriptor)); ret != nil {
			if cb.wrap {
				switch d.StreamType {
				case logpb.StreamType_TEXT:
					ret = getWrappedTextCallback(ret)
				case logpb.StreamType_DATAGRAM:
					ret = getWrappedDatagramCallback(ret)
				}
			}
			b.streamCallbacksMu.Lock()
			defer b.streamCallbacksMu.Unlock()
			b.streamCallbacks[d.Name] = ret
			return
		}
	}
}

func (b *Butler) runCallbacks(bundle *logpb.ButlerLogBundle) {
	b.streamCallbacksMu.RLock()
	cbsCopy := make(map[string]StreamChunkCallback, len(b.streamCallbacks))
	for k, v := range b.streamCallbacks {
		cbsCopy[k] = v
	}
	b.streamCallbacksMu.RUnlock()

	for _, entry := range bundle.Entries {
		name := entry.Desc.Name
		cb := cbsCopy[name]
		if cb != nil {
			for _, log := range entry.Logs {
				cb(log)
			}
		}
		if entry.Terminal {
			b.finalCallback(name)
		}
	}
}

// finalCallback is called from two places.
//
// Once, from runStreams, in the event that the stream read NO data from the
// user.
//
// The other, from runCallbacks, in the event that the bundler produced the
// terminal index for the stream.
//
// Ideally this would ONLY be used from runCallbacks... however bundler treats
// streams-with-data and streams-without-data asymmetrically.
//
// TODO(iannucci): Make ALL streams appear via bundler, even empty ones.
func (b *Butler) finalCallback(name string) {
	b.streamCallbacksMu.Lock()
	cb, ok := b.streamCallbacks[name]
	if ok {
		delete(b.streamCallbacks, name)
	}
	b.streamCallbacksMu.Unlock()
	if cb != nil {
		cb(nil)
	}
	b.streams.DrainedStream(types.StreamName(name))
}
