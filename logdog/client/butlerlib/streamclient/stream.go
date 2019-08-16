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

package streamclient

import (
	"errors"
	"io"

	"github.com/golang/protobuf/proto"
	"go.chromium.org/luci/common/data/recordio"
	"go.chromium.org/luci/logdog/api/logpb"
)

// Stream is an individual LogDog Butler stream.
type Stream interface {
	io.WriteCloser

	// WriteDatagram writes a LogDog Butler streaming datagram to the underlying
	// Writer.
	WriteDatagram([]byte) error

	// Descriptor returns a copy of this Stream's descriptor.
	Descriptor() *logpb.LogStreamDescriptor
}

// BaseStream is the standard implementation of the Stream interface.
type BaseStream struct {
	// WriteCloser (required) is the stream's underlying io.WriteCloser.
	io.WriteCloser

	// D (required) is this stream's descriptor.
	D *logpb.LogStreamDescriptor

	// rioW is a recordio.Writer bound to the WriteCloser. This will be
	// initialized on the first writeRecord invocation.
	rioW recordio.Writer
}

var _ Stream = (*BaseStream)(nil)

// WriteDatagram implements StreamClient.
func (s *BaseStream) WriteDatagram(dg []byte) error {
	if !s.isDatagramStream() {
		return errors.New("not a datagram stream")
	}

	return s.writeRecord(dg)
}

// Write implements StreamClient.
func (s *BaseStream) Write(data []byte) (int, error) {
	if s.isDatagramStream() {
		return 0, errors.New("cannot use Write with datagram stream")
	}

	return s.writeRaw(data)
}

func (s *BaseStream) writeRaw(data []byte) (int, error) {
	return s.WriteCloser.Write(data)
}

func (s *BaseStream) writeRecord(r []byte) error {
	if s.rioW == nil {
		s.rioW = recordio.NewWriter(s.WriteCloser)
	}
	if _, err := s.rioW.Write(r); err != nil {
		return err
	}
	return s.rioW.Flush()
}

func (s *BaseStream) isDatagramStream() bool {
	return s.D.StreamType == logpb.StreamType_DATAGRAM
}

// Descriptor implements StreamClient.
func (s *BaseStream) Descriptor() *logpb.LogStreamDescriptor {
	return proto.Clone(s.D).(*logpb.LogStreamDescriptor)
}
