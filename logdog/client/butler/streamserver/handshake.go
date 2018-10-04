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

package streamserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/recordio"
	"go.chromium.org/luci/common/iotools"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
)

// handshakeProtocol is an implementation of a Butler handshake protocol V1
// reader. It identifies with streamproto.ProtocolFrameHeaderMagic, and uses a
// JSON blob to describe the stream.
type handshakeProtocol struct {
	forceVerbose bool // (Testing) force verbose code path.
}

const (
	// The maximum size of the header (1MB).
	maxHeaderSize = 1 * 1024 * 1024
)

func (p *handshakeProtocol) defaultFlags() *streamproto.Flags {
	return &streamproto.Flags{
		Type: streamproto.StreamType(logpb.StreamType_TEXT),
		Tee:  streamproto.TeeNone,
	}
}

func (p *handshakeProtocol) Handshake(ctx context.Context, r io.Reader) (*streamproto.Properties, error) {
	// Read the frame header magic number (version)
	magic := make([]byte, len(streamproto.ProtocolFrameHeaderMagic))
	if n, err := io.ReadFull(r, magic); (n != len(magic)) || (err != nil) {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(ctx, "Failed to read frame header magic number.")
		return nil, errors.New("handshake: failed to read frame header magic number")
	}

	// Check the magic number/version
	if !bytes.Equal(magic, streamproto.ProtocolFrameHeaderMagic) {
		log.Fields{
			"magic": fmt.Sprintf("%#X", magic),
		}.Errorf(ctx, "Unrecognized frame header magic number.")
		return nil, errors.New("handshake: Unknown protocol magic in frame header")
	}

	// Load the JSON into our descriptor field.
	flags, err := p.loadFlags(ctx, r)
	if err != nil {
		return nil, err
	}

	props := flags.Properties()
	if props.Timestamp == nil {
		props.Timestamp = google.NewTimestamp(clock.Now(ctx))
	}
	if err := props.Validate(); err != nil {
		return nil, err
	}

	return props, nil
}

func (p *handshakeProtocol) loadFlags(ctx context.Context, r io.Reader) (*streamproto.Flags, error) {
	fr := recordio.NewReader(r, maxHeaderSize)

	// Read the header frame.
	frameSize, hr, err := fr.ReadFrame()
	if err != nil {
		log.Errorf(log.SetError(ctx, err), "Failed to read header frame.")
		return nil, err
	}

	// When tracing, buffer the JSON data locally so we can emit it via log.
	headerBuf := bytes.Buffer{}
	captureHeader := log.IsLogging(ctx, log.Debug) || p.forceVerbose
	if captureHeader {
		hr = io.TeeReader(hr, &headerBuf)
	}

	// When we hand the header reader to the "json" library, we want to count how
	// many bytes it reads from it. We will assert that it has read the full set
	// of bytes.
	chr := &iotools.CountingReader{Reader: hr}

	// Decode into our protocol description structure. Note that extra fields
	// are ignored (no error) and missing fields retain their zero value.
	f := p.defaultFlags()
	err = json.NewDecoder(chr).Decode(f)
	if captureHeader {
		log.Fields{
			"frameSize":  frameSize,
			"decodeSize": headerBuf.Len(),
		}.Debugf(ctx, "Read JSON header:\n%s", headerBuf.String())
	}
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(ctx, "Failed to decode stream description data JSON.")
		return nil, err
	}

	// Make sure that this consumed the full JSON size that was specified.
	//
	// We use a countReader because the 'json' library doesn't give us a way to
	// know how many bytes it consumed when it decoded.
	if chr.Count != frameSize {
		log.Fields{
			"blockSize": chr.Count,
			"frameSize": frameSize,
		}.Errorf(ctx, "Stream description block was not fully consumed.")
		return nil, errors.New("handshake: stream description block was not fully consumed")
	}

	return f, nil
}
