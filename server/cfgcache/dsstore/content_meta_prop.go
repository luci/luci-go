// Copyright 2026 The LUCI Authors.
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

package dsstore

import (
	"bytes"
	"encoding/base64"
	"fmt"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"

	cfgcachepb "go.chromium.org/luci/server/cfgcache/proto"
)

// contentMetaProperty renders to:
//
// "base64URL(PipelineID)|base64URL(Meta)"
//
// Including base64URL(PipelineID) as the prefix allows LoadMeta to execute a
// range scan on the built-in single-property "Meta" index while projecting
// "Meta" (since Datastore's built-in property index is ordered by
// (Meta ASC, __key__ ASC)).
type contentMetaProperty struct {
	PipelineID string
	*cfgcachepb.Meta
}

var _ datastore.PropertyConverter = (*contentMetaProperty)(nil)

func (c contentMetaProperty) String() string {
	// NOTE: This struct must be used as a value (not a pointer) for the
	// datastore library to properly handle the PropertyConverter protocol; we
	// use a value receiver just for String() to allow this to be printed nicely
	// in logs/errors without needing to take its address.
	metaEnc, err := proto.Marshal(c.Meta)
	if err != nil {
		panic(err)
	}

	return fmt.Sprintf("%s|%s",
		base64.RawURLEncoding.EncodeToString([]byte(c.PipelineID)),
		base64.RawURLEncoding.EncodeToString(metaEnc))
}

func (c *contentMetaProperty) ToProperty() (datastore.Property, error) {
	return datastore.MkProperty(c.String()), nil
}

func (c *contentMetaProperty) FromProperty(p datastore.Property) error {
	raw, err := p.Project(datastore.PTBytes)
	if err != nil {
		return err
	}
	toks := bytes.Split(raw.([]byte), []byte("|"))
	if len(toks) != 2 {
		return errors.Fmt("contentMetaProperty: expected 2 tokens: %q", raw)
	}
	id, err := base64.RawURLEncoding.AppendDecode(nil, toks[0])
	if err != nil {
		return err
	}
	metaEnc, err := base64.RawURLEncoding.AppendDecode(nil, toks[1])
	if err != nil {
		return err
	}
	meta := &cfgcachepb.Meta{}
	if err := proto.Unmarshal(metaEnc, meta); err != nil {
		return err
	}
	c.PipelineID = string(id)
	c.Meta = meta
	return nil
}
