// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package model

import (
	"bytes"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/data/cmpbin"
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
