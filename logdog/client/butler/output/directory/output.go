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

package directory

import (
	"context"
	"sync"

	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butler/output"
	"go.chromium.org/luci/logdog/common/types"
)

// Options should be used to configure and make a directory Output (using the
// .New() method).
type Options struct {
	// Path is the output base path.
	//
	// All opened streams will be written to disk relative to this directory.
	//
	// All stream metadata will be written to 'path/of/stream/.meta.name' as JSON.
	//
	// Datagram streams will be written as 'path/of/stream/.N.name' where N is the
	// index of the datagram in the stream (i.e. 0 is the first datagram, etc.)
	Path string
}

// New creates a new file Output from the specified Options.
func (opt Options) New(c context.Context) output.Output {
	o := dirOutput{
		Context: c,
		Options: &opt,
		streams: map[types.StreamPath]*stream{},
	}
	return &o
}

// dirOutput is an output.Output implementation that writes stream data to
// on-disk files.
type dirOutput struct {
	// Context is the context to use for logging. If nil, no logging will be
	// performed.
	context.Context
	// Options are the configuration options.
	*Options
	// Mutex protects all other members.
	sync.Mutex

	// streams is a map of stream name to stream handler.
	streams map[types.StreamPath]*stream
}

func (o *dirOutput) SendBundle(b *logpb.ButlerLogBundle) error {
	o.Lock()
	defer o.Unlock()

	for _, be := range b.GetEntries() {
		desc := be.GetDesc()
		if desc == nil {
			continue
		}
		path := desc.Path()

		s, ok := o.streams[path]
		if !ok {
			var err error
			s, err = newStream(o.Path, desc)
			if err != nil {
				return err
			}
			o.streams[path] = s
		}

		done, err := s.ingestBundleEntry(be)
		if err != nil {
			return err
		}
		if done {
			delete(o.streams, path)
		}
	}

	return nil
}

func (o *dirOutput) Close() {
	o.Lock()
	defer o.Unlock()

	if o.streams == nil {
		panic("already closed")
	}
	for _, stream := range o.streams {
		stream.Close()
	}
	o.streams = nil
}

func (o *dirOutput) MaxSize() int                { return 1024 * 1024 * 1024 }
func (o *dirOutput) Stats() output.Stats         { return nil }
func (o *dirOutput) Record() *output.EntryRecord { return nil }
