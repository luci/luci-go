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

package archive

import (
	"io"

	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/renderer"
)

// channelRendererSource is a renderer.Source implementation that reads LogEntry
// records from a channel. It also 'sticks' on the first error that it
// encounters, so that all subsequent interface methods will permanently return
// the first encountered error.
//
// When the channel has closed, it will return io.EOF conformant to
// renderer.Source.
type channelRendererSource <-chan *logpb.LogEntry

func (cs channelRendererSource) NextLogEntry() (*logpb.LogEntry, error) {
	if le, ok := <-cs; ok {
		return le, nil
	}
	return nil, io.EOF
}

func archiveData(w io.Writer, dataC <-chan *logpb.LogEntry) error {
	entryC := make(chan *logpb.LogEntry)
	r := renderer.Renderer{
		Source: channelRendererSource(entryC),
		Raw:    true,
	}

	// Run our Renderer in a goroutine.
	rendererFinishedC := make(chan error)
	go func() {
		var err error
		defer func() {
			rendererFinishedC <- err
		}()

		// Render our stream.
		_, err = io.Copy(w, &r)

		// If our Renderer encounters an error, it will stop reading from entryC
		// prematurely. In order for the pipeline to complete, we need to manually
		// drain entryC here.
		for range entryC {
			// Discard.
		}
	}()

	// Consume records from dataC. If we hit an error condition, consume and
	// discard.
	for le := range dataC {
		entryC <- le
	}
	close(entryC)
	return <-rendererFinishedC
}
