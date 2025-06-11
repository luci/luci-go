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

// Package output contains helpers for emitting structure JSON output.
package output

import (
	"bytes"
	"encoding/json"
	"io"
	"sort"
	"sync"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
)

// marshalOpts is how to JSON marshal proto messages.
var marshalOpts = protojson.MarshalOptions{UseProtoNames: true}

// streamingMode defines what kind of output Sink is streaming right now.
type streamingMode int

const (
	// Not known yet, the initial state.
	unknown streamingMode = 0
	// Not really streaming, just writing a single blob at once.
	oneShot streamingMode = 1
	// Streaming a list of items. The overall output is a JSON list.
	streamList streamingMode = 2
	// Streaming a list of key-value pairs. The overall output is a JSON map.
	streamMap streamingMode = 3
)

// Sink is what eventually receives the output.
//
// Safe for concurrent use.
type Sink struct {
	discard bool // if true, skip all writes

	m     sync.Mutex
	mode  streamingMode // what sort of object we are writing
	calls int           // number of appendJSON calls made so far
	buf   bytes.Buffer  // used as an internal scratch space
	w     io.Writer     // receives formatted JSON chunk by chunk
}

// NewSink returns a new sink that writes JSON to the given output.
//
// Must be finalize in the end with Finalize. Note that it takes an io.Writer,
// not an io.WriterCloser. The caller will still need to close the output
// stream.
func NewSink(out io.Writer) *Sink {
	return &Sink{w: out}
}

// NewDiscardingSink returns a sink that just quickly discards all writes.
func NewDiscardingSink() *Sink {
	return &Sink{discard: true}
}

// Finalize writes the JSON syntax to properly close the list or the map.
func (s *Sink) Finalize() error {
	if s.discard {
		return nil
	}
	s.m.Lock()
	defer s.m.Unlock()
	var err error
	switch s.mode {
	case oneShot:
		_, err = s.w.Write([]byte("\n"))
	case streamList:
		if s.calls == 0 {
			_, err = s.w.Write([]byte("[]\n"))
		} else {
			_, err = s.w.Write([]byte("\n]\n"))
		}
	case streamMap:
		if s.calls == 0 {
			_, err = s.w.Write([]byte("{}\n"))
		} else {
			_, err = s.w.Write([]byte("\n}\n"))
		}
	}
	return err
}

// Proto writes a single protobuf message as the entire output.
//
// It uses protojson encoding to serialize it.
func Proto(sink *Sink, msg proto.Message) error {
	if sink.discard {
		return nil
	}
	blob, err := marshalOpts.Marshal(msg)
	if err != nil {
		return err
	}
	return sink.appendJSON(oneShot, "", blob)
}

// JSON writes a single JSON-marshallable object as the entire output.
//
// It uses the standard Go library JSON encoder to serialize it.
func JSON(sink *Sink, obj any) error {
	if sink.discard {
		return nil
	}
	blob, err := json.MarshalIndent(obj, "", " ")
	if err != nil {
		return err
	}
	return sink.appendJSON(oneShot, "", blob)
}

// List writes a JSON list where each item is a proto message.
//
// This is a helper that just calls StartList and ListEntry.
func List[T proto.Message](sink *Sink, items []T) error {
	if sink.discard {
		return nil
	}
	if err := StartList(sink); err != nil {
		return err
	}
	for _, v := range items {
		if err := ListEntry(sink, v); err != nil {
			return err
		}
	}
	return nil
}

// StartList switches the writer to emitting a list of items.
func StartList(sink *Sink) error {
	if sink.discard {
		return nil
	}
	return sink.setMode(streamList)
}

// ListEntry writes a single list entry to the output.
//
// It can be called multiple times to write many entries one by one.
func ListEntry[T proto.Message](sink *Sink, entry T) error {
	if sink.discard {
		return nil
	}
	blob, err := marshalOpts.Marshal(entry)
	if err != nil {
		return err
	}
	return sink.appendJSON(streamList, "", blob)
}

// Map writes a JSON map where each item is a proto message.
//
// This is a helper that just calls StartMap and MapEntry. Sorts map entries by
// key.
func Map[T proto.Message](sink *Sink, items map[string]T) error {
	if sink.discard {
		return nil
	}

	keys := make([]string, 0, len(items))
	for k := range items {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	if err := StartMap(sink); err != nil {
		return err
	}
	for _, k := range keys {
		if err := MapEntry(sink, k, items[k]); err != nil {
			return err
		}
	}
	return nil
}

// StartMap switches the writer to emitting a map of items.
func StartMap(sink *Sink) error {
	if sink.discard {
		return nil
	}
	return sink.setMode(streamMap)
}

// MapEntry writes a single map entry to the output.
//
// It can be called multiple times to write many entries one by one.
func MapEntry[T proto.Message](sink *Sink, key string, entry T) error {
	if sink.discard {
		return nil
	}
	blob, err := marshalOpts.Marshal(entry)
	if err != nil {
		return err
	}
	return sink.appendJSON(streamMap, key, blob)
}

// setMode switches the sink into some streaming mode if not yet switched.
func (s *Sink) setMode(mode streamingMode) error {
	s.m.Lock()
	defer s.m.Unlock()
	return s.setModeLocked(mode)
}

// setModeLocked must be called when the lock held.
func (s *Sink) setModeLocked(mode streamingMode) error {
	if s.mode != unknown && s.mode != mode {
		return errors.New("incorrect usage of output writer API: cannot switch streaming modes midway")
	}
	if mode == oneShot && s.calls > 0 {
		return errors.New("incorrect usage of output writer API: cannot emit more than one message while not in a list or map mode")
	}
	s.mode = mode
	return nil
}

// appendJSON appends the given JSON to the output.
//
// `mode` defines the overall format of the output stream. It can't change, i.e.
// once a call to `appendJSON(mode, ...)` has been made, all subsequent calls
// should use the same mode (because we can't start writing a JSON list and then
// continue writing it as a JSON map).
//
// Reformats JSON to look pretty and to use consistent indentation (to
// workaround https://github.com/golang/protobuf/issues/1082).
func (s *Sink) appendJSON(mode streamingMode, key string, jsonBlob []byte) error {
	s.m.Lock()
	defer s.m.Unlock()

	s.buf.Reset()

	// Make sure we don't mix list and map outputs or emit more than one JSON
	// object.
	if err := s.setModeLocked(mode); err != nil {
		return err
	}
	s.calls++

	// Emit JSON syntax to start or advance the output.
	if s.calls == 1 {
		switch mode {
		case streamList:
			_, _ = s.buf.Write([]byte("[\n "))
		case streamMap:
			_, _ = s.buf.Write([]byte("{\n "))
		}
	} else {
		_, _ = s.buf.Write([]byte(",\n "))
	}

	// For maps, write `"<key>": ` prefix first.
	if mode == streamMap {
		keyJSON, err := json.Marshal(key)
		if err != nil {
			return err
		}
		_, _ = s.buf.Write(keyJSON)
		_, _ = s.buf.Write([]byte(": "))
	}

	// When writing maps or lists need to indent the message one level. In all
	// cases need to reformat `jsonBlob` to use consistent indentation levels.
	prefix := ""
	if mode == streamList || mode == streamMap {
		prefix = " "
	}
	if err := json.Indent(&s.buf, jsonBlob, prefix, " "); err != nil {
		return err
	}

	// Flush the buffered output to the actual sink.
	defer s.buf.Reset()
	_, err := io.Copy(s.w, &s.buf)
	return err
}
