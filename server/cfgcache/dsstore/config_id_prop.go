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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"
)

// cachedConfigIDProperty renders to:
//
// "base64URL(PipelineID)|base64URL(ConfigSet)"
//
// This encoding was chosen to respect 'rules' of utf8 compatible names and no
// slashes (otherwise I would have used cmpbin).
type cachedConfigIDProperty struct {
	PipelineID string
	ConfigSet  config.Set
}

var _ datastore.PropertyConverter = (*cachedConfigIDProperty)(nil)

func (c cachedConfigIDProperty) String() string {
	// NOTE: This struct must be used as a value (not a pointer) for the
	// datastore library to properly handle the PropertyConverter protocol; we
	// use a value receiver just for String() to allow this to be printed nicely
	// in logs/errors without needing to take its address.
	return fmt.Sprintf("%s|%s",
		base64.RawURLEncoding.EncodeToString([]byte(c.PipelineID)),
		base64.RawURLEncoding.EncodeToString([]byte(c.ConfigSet)),
	)
}

func (c *cachedConfigIDProperty) ToProperty() (datastore.Property, error) {
	return datastore.MkProperty(c.String()), nil
}

func (c *cachedConfigIDProperty) FromProperty(p datastore.Property) error {
	raw, err := p.Project(datastore.PTBytes)
	if err != nil {
		return err
	}
	toks := bytes.Split(raw.([]byte), []byte("|"))
	if len(toks) != 2 {
		return errors.Fmt("cachedConfigIDProperty: expected 2 tokens: %q", raw)
	}
	pipelineID, err1 := base64.RawURLEncoding.AppendDecode(nil, toks[0])
	configSet, err2 := base64.RawURLEncoding.AppendDecode(nil, toks[1])
	if err := errors.Join(err1, err2); err != nil {
		return err
	}
	c.PipelineID = string(pipelineID)
	c.ConfigSet = config.Set(configSet)
	return nil
}
