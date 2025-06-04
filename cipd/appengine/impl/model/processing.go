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

package model

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"io"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
)

// ProcessingResult holds information extracted from the package instance file.
//
// It is obtained during an asynchronous post processing step triggered after
// the instance is uploaded. Immutable.
//
// Entity ID is a processor name used to extract it. Parent entity is
// PackageInstance the information was extracted from.
type ProcessingResult struct {
	_kind  string                `gae:"$kind,ProcessingResult"`
	_extra datastore.PropertyMap `gae:"-,extra"`

	ProcID   string         `gae:"$id"`     // processor that generated the result
	Instance *datastore.Key `gae:"$parent"` // instance it was generated from

	CreatedTs time.Time `gae:"created_ts"`     // when it was generated
	Success   bool      `gae:"success"`        // mostly for for indexing
	Error     string    `gae:"error,noindex"`  // for Success == false
	ResultRaw []byte    `gae:"result,noindex"` // for Success == true
}

// WriteResult overwrites ResultRaw field with compressed JSON-serialized 'r'.
//
// 'r' should serialize into a JSON object, e.g. '{...}'.
func (p *ProcessingResult) WriteResult(r any) error {
	blob, err := json.Marshal(r)
	switch {
	case err != nil:
		return errors.Fmt("failed to serialize the result: %w", err)
	case len(blob) == 0 || blob[0] != '{':
		return errors.New("the result is not a JSON object")
	}
	out := bytes.Buffer{}
	z := zlib.NewWriter(&out)
	if _, err := io.Copy(z, bytes.NewReader(blob)); err != nil {
		z.Close()
		return errors.Fmt("failed to compress the result: %w", err)
	}
	if err := z.Close(); err != nil {
		return errors.Fmt("failed to close zlib writer: %w", err)
	}
	p.ResultRaw = out.Bytes()
	return nil
}

// ReadResult deserializes the result into the given variable.
//
// Does nothing if there's no results stored.
func (p *ProcessingResult) ReadResult(r any) error {
	if len(p.ResultRaw) == 0 {
		return nil
	}
	z, err := zlib.NewReader(bytes.NewReader(p.ResultRaw))
	if err != nil {
		return errors.Fmt("failed to open the blob for zlib decompression: %w", err)
	}
	if err := json.NewDecoder(z).Decode(r); err != nil {
		z.Close()
		return errors.Fmt("failed to decompress or deserialize the result: %w", err)
	}
	if err := z.Close(); err != nil {
		return errors.Fmt("failed to close zlib reader: %w", err)
	}
	return nil
}

// ReadResultIntoStruct deserializes the result into the protobuf.Struct.
//
// Does nothing if there's no results stored.
func (p *ProcessingResult) ReadResultIntoStruct(s *structpb.Struct) error {
	if len(p.ResultRaw) == 0 {
		return nil
	}
	z, err := zlib.NewReader(bytes.NewReader(p.ResultRaw))
	if err != nil {
		return errors.Fmt("failed to open the blob for zlib decompression: %w", err)
	}
	blob, err := io.ReadAll(z)
	if err != nil {
		z.Close()
		return errors.Fmt("failed to decompress the result: %w", err)
	}
	if err := z.Close(); err != nil {
		return errors.Fmt("failed to close zlib reader: %w", err)
	}
	if err := protojson.Unmarshal(blob, s); err != nil {
		return errors.Fmt("failed to deserialize the result: %w", err)
	}
	return nil
}

// Proto returns cipd.Processor proto with information from this entity.
func (p *ProcessingResult) Proto() (*api.Processor, error) {
	out := &api.Processor{Id: p.ProcID}

	if p.CreatedTs.IsZero() {
		out.State = api.Processor_PENDING // no result yet
		return out, nil
	}
	out.FinishedTs = timestamppb.New(p.CreatedTs)

	if p.Success {
		out.State = api.Processor_SUCCEEDED
		res := &structpb.Struct{}
		if err := p.ReadResultIntoStruct(res); err != nil {
			return nil, err
		}
		if len(res.Fields) != 0 {
			out.Result = res
		}
	} else {
		out.State = api.Processor_FAILED
		out.Error = p.Error
	}

	return out, nil
}
