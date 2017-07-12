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

package recordio

import (
	"bytes"
	"encoding/binary"
	"io"
)

func writeFrameHeader(w io.Writer, frameSize uint64) (int, error) {
	sizeBuf := make([]byte, binary.MaxVarintLen64)
	return w.Write(sizeBuf[:binary.PutUvarint(sizeBuf, frameSize)])
}

// WriteFrame writes a single frame to an io.Writer.
func WriteFrame(w io.Writer, frame []byte) (int, error) {
	count, err := writeFrameHeader(w, uint64(len(frame)))
	if err != nil {
		return count, err
	}

	amount, err := w.Write(frame)
	count += amount
	if err != nil {
		return count, err
	}

	return count, nil
}

// Writer implements the io.Writer interface. Data written to the Writer is
// translated into a series of frames. Each frame is spearated by a call to
// Flush.
//
// Frame boundaries are created by calling Flush. Flush will always write a
// frame, even if the frame's data size is zero.
//
// Data written over consecutive Write calls belongs to the same frame. It is
// buffered until a frame boundary is created via Flush().
type Writer interface {
	io.Writer

	// Flush writes the buffered frame
	Flush() error

	// Reset clears the writer state and attaches it to a new inner Writer
	// instance.
	Reset(io.Writer)
}

// writer implements the Writer interface by wrapping an io.Writer.
type writer struct {
	inner io.Writer
	buf   bytes.Buffer
}

// NewWriter creates a new Writer instance that data as frames to an underlying
// io.Writer.
func NewWriter(w io.Writer) Writer {
	return &writer{
		inner: w,
	}
}

func (w *writer) Write(data []byte) (int, error) {
	return w.buf.Write(data)
}

func (w *writer) Flush() error {
	_, err := WriteFrame(w.inner, w.buf.Bytes())
	if err != nil {
		return err
	}

	w.buf.Reset()
	return nil
}

func (w *writer) Reset(inner io.Writer) {
	w.inner = inner
	w.buf.Reset()
}
