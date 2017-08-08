// Copyright 2017 The LUCI Authors.
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

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/data/cmpbin"
)

// ManifestLink is an in-MILO link to a named source manifest.
type ManifestLink struct {
	// The name of the manifest as the build annotated it.
	Name string

	// The manifest ID (sha256).
	ID []byte
}

var _ ds.PropertyConverter = (*ManifestLink)(nil)

// FromProperty implements ds.PropertyConverter
func (m *ManifestLink) FromProperty(p ds.Property) (err error) {
	val, err := p.Project(ds.PTBytes)
	if err != nil {
		return err
	}
	buf := bytes.NewReader(val.([]byte))
	if m.Name, _, err = cmpbin.ReadString(buf); err != nil {
		return
	}
	m.ID, _, err = cmpbin.ReadBytes(buf)
	return
}

// ToProperty implements ds.PropertyConverter
func (m *ManifestLink) ToProperty() (ds.Property, error) {
	buf := bytes.NewBuffer(nil)
	cmpbin.WriteString(buf, m.Name)
	cmpbin.WriteBytes(buf, m.ID)
	return ds.MkProperty(buf.Bytes()), nil
}
