// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package types

import (
	"fmt"
	"math"

	"github.com/luci/gae/service/datastore"
)

// UInt32 is a datastore.PropertyConverter compatible uint32 field, since
// datastore doesn't support unsigned numbers.
type UInt32 uint32

var _ datastore.PropertyConverter = (*UInt32)(nil)

// FromProperty implements datastore.PropertyConverter
func (u *UInt32) FromProperty(p datastore.Property) error {
	val, err := p.Project(datastore.PTInt)
	if err != nil {
		return err
	}
	ival := val.(int64)
	if ival < 0 || ival > math.MaxUint32 {
		return fmt.Errorf("uint32 out of bounds: %d", ival)
	}

	*u = UInt32(ival)
	return nil
}

// ToProperty implements datastore.PropertyConverter
func (u *UInt32) ToProperty() (datastore.Property, error) {
	return datastore.MkPropertyNI(int64(*u)), nil
}
