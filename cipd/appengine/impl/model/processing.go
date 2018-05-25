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
	"time"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"
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
func (p *ProcessingResult) WriteResult(r interface{}) error {
	out := bytes.Buffer{}
	z := zlib.NewWriter(&out)
	if err := json.NewEncoder(z).Encode(r); err != nil {
		z.Close()
		return errors.Annotate(err, "failed to serialize or compress the result").Err()
	}
	if err := z.Close(); err != nil {
		return errors.Annotate(err, "failed to close zlib writer").Err()
	}
	p.ResultRaw = out.Bytes()
	return nil
}

// ReadResult reads result from the entity (decompressing and deserializing it).
//
// Does nothing if there's no results stored.
func (p *ProcessingResult) ReadResult(r interface{}) error {
	if len(p.ResultRaw) == 0 {
		return nil
	}
	z, err := zlib.NewReader(bytes.NewReader(p.ResultRaw))
	if err != nil {
		return errors.Annotate(err, "failed to open the blob for zlib decompression").Err()
	}
	if err := json.NewDecoder(z).Decode(r); err != nil {
		z.Close()
		return errors.Annotate(err, "failed to decompress or deserialize the result").Err()
	}
	if err := z.Close(); err != nil {
		return errors.Annotate(err, "failed to close zlib reader").Err()
	}
	return nil
}
