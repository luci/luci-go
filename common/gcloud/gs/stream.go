// Copyright 2024 The LUCI Authors.
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

package gs

import (
	"bufio"
	"context"
	"time"
)

// ObjectStreamOptions configure the behavior of an [ObjectStream].
//
// PollFrequency controls how frequently GCS object at Path is checked
// for updates once you reach EOF.
//
// LinesC and ErrorsC are used to return lines of object at Path and any
// errors encountered. If nothing reads these channels sending will block.
type ObjectStreamOptions struct {
	Client        Client
	Path          Path
	PollFrequency time.Duration
	LinesC        chan<- string
	ErrorsC       chan<- error
}

// ObjectStream lets you stream a GCS object as lines. This is useful
// when you are trying to stream something like a logs file that is
// being updated as you are reading it. It assumes the object always
// has complete lines.
//
// [ObjectStreamOptions] has some guidance on how to use it.
//
// TODO(b/332706067): Deprecate this.
type ObjectStream struct {
	ctx     context.Context
	options *ObjectStreamOptions

	cursor int64
	closec chan struct{}
}

// NewObjectStream creates an [ObjectStream] and starts a background routine
// that starts streaming the file.
func NewObjectStream(ctx context.Context, options *ObjectStreamOptions) *ObjectStream {
	os := &ObjectStream{
		ctx:     ctx,
		options: options,
		closec:  make(chan struct{}),
	}
	go os.streamWorker()
	return os
}

// Close stops the background routine.
func (os *ObjectStream) Close() error {
	close(os.closec)
	return nil
}

func (os *ObjectStream) streamWorker() {
	for {
		select {
		case <-os.ctx.Done():
			return
		case <-os.closec:
			return
		default:
			if err := os.poll(); err != nil {
				os.options.ErrorsC <- err
			}
			time.Sleep(os.options.PollFrequency)
		}
	}
}

func (os *ObjectStream) poll() error {
	attrs, err := os.options.Client.Attrs(os.options.Path)
	if err != nil {
		return err
	}
	if os.cursor >= attrs.Size {
		return nil
	}

	r, err := os.options.Client.NewReader(os.options.Path, os.cursor, -1)
	if err != nil {
		return err
	}

	s := bufio.NewScanner(r)
	for s.Scan() {
		line := s.Text()
		os.options.LinesC <- line
		// Extra 1 to account for the new line character
		// that the scanner eats
		os.cursor += int64(len(line) + 1)
	}
	return s.Err()
}
