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

package streamproto

import (
	"bytes"
	"encoding/json"
	"io"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/clockflag"
	"go.chromium.org/luci/common/data/recordio"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/types"
)

// Flags is a flag- and JSON-compatible version of logpb.LogStreamDescriptor.
// It is used for stream negotiation protocol and command-line interfaces.
//
// TODO(iannucci) - Change client->butler protocol to just use jsonpb encoding
// of LogStreamDescriptor.
type Flags struct {
	Name        StreamNameFlag `json:"name,omitempty"`
	ContentType string         `json:"contentType,omitempty"`
	Type        StreamType     `json:"type,omitempty"`
	Timestamp   clockflag.Time `json:"timestamp,omitempty"`
	Tags        TagMap         `json:"tags,omitempty"`
}

// Descriptor converts the Flags to a LogStreamDescriptor.
func (f *Flags) Descriptor() *logpb.LogStreamDescriptor {
	contentType := types.ContentType(f.ContentType)
	if contentType == "" {
		contentType = f.Type.DefaultContentType()
	}

	var t *timestamppb.Timestamp
	if !f.Timestamp.Time().IsZero() {
		t = timestamppb.New(f.Timestamp.Time())
	}

	return &logpb.LogStreamDescriptor{
		Name:        string(f.Name),
		ContentType: string(contentType),
		StreamType:  logpb.StreamType(f.Type),
		Timestamp:   t,
		Tags:        f.Tags,
	}
}

// The maximum size of the initial header in bytes that FromHandshake is willing
// to read.
//
// This must include all fields of the Flags; We're counting on the total
// encoded size of this being less than 1MiB.
const maxFrameSize = 1024 * 1024

// WriteHandshake writes the butler protocol header handshake on the given
// Writer.
func (f *Flags) WriteHandshake(w io.Writer) error {
	data, err := json.Marshal(f)
	if err != nil {
		return errors.Annotate(err, "marshaling flags").Err()
	}
	if _, err := w.Write(ProtocolFrameHeaderMagic); err != nil {
		return errors.Annotate(err, "writing magic number").Err()
	}
	if _, err := recordio.WriteFrame(w, data); err != nil {
		return errors.Annotate(err, "writing properties").Err()
	}
	return nil
}

// FromHandshake reads the butler protocol header handshake from the given
// Reader.
func (f *Flags) FromHandshake(r io.Reader) error {
	header := make([]byte, len(ProtocolFrameHeaderMagic))
	_, err := io.ReadFull(r, header)
	if err != nil {
		return errors.Annotate(err, "reading magic number").Err()
	}
	if !bytes.Equal(header, ProtocolFrameHeaderMagic) {
		return errors.Reason(
			"magic number mismatch: got(%q) expected(%q)",
			header, ProtocolFrameHeaderMagic).Err()
	}

	_, frameReader, err := recordio.NewReader(r, maxFrameSize).ReadFrame()
	if err != nil {
		return errors.Annotate(err, "reading property frame").Err()
	}

	if err := json.NewDecoder(frameReader).Decode(f); err != nil {
		return errors.Annotate(err, "parsing flag JSON").Err()
	}

	if frameReader.N > 0 {
		return errors.Reason("handshake had %d bytes of trailing data", frameReader.N).Err()
	}

	return nil
}
