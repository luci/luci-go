// Copyright 2023 The LUCI Authors.
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

package datastore

import (
	"fmt"
	"reflect"
	"time"

	"golang.org/x/exp/constraints"
)

// Indexed indicates to Optional or Nullable to produce indexed properties.
type Indexed struct{}

// Unindexed indicates to Optional or Nullable to produce unindexed properties.
type Unindexed struct{}

// Indexing is implemented by Indexed and Unindexed.
type Indexing interface{ shouldIndex() IndexSetting }

func (Indexed) shouldIndex() IndexSetting   { return ShouldIndex }
func (Unindexed) shouldIndex() IndexSetting { return NoIndex }

// Elementary is a type set with all "elementary" datastore types.
type Elementary interface {
	constraints.Integer | constraints.Float | ~bool | ~string | ~[]byte | time.Time | GeoPoint | *Key
}

// Optional wraps an elementary property type, adding "is set" flag to it.
//
// A pointer to Optional[T, I] implements PropertyConverter, allowing values of
// Optional[T, I] to appear as fields in structs representing entities.
//
// This is useful for rare cases when it is necessary to distinguish a zero
// value of T from an absent value. For example, a zero integer property ends up
// in indices, but an absent property doesn't.
//
// A zero value of Optional[T, I] represents an unset property. Setting a value
// via Set(...) (even if this is a zero value of T) marks the property as set.
//
// Unset properties are not stored into the datastore at all and they are
// totally invisible to all queries. Conversely, when an entity is being loaded,
// its Optional[T, I] fields that don't match any loaded properties will remain
// in unset state. Additionally, PTNull properties are treated as unset as well.
//
// To store unset properties as PTNull, use Nullable[T, I] instead. The primary
// benefit is the ability to filter queries by null, i.e. absence of a property.
//
// Type parameter I controls if the stored properties should be indexed or
// not. It should either be Indexed or Unindexed.
type Optional[T Elementary, I Indexing] struct {
	isSet bool
	val   T
}

var _ PropertyConverter = &Optional[bool, Indexed]{}

// NewIndexedOptional creates a new, already set, indexed optional.
//
// To get an unset optional, just use the zero of Optional[T, I].
func NewIndexedOptional[T Elementary](val T) Optional[T, Indexed] {
	return Optional[T, Indexed]{isSet: true, val: val}
}

// NewUnindexedOptional creates a new, already set, unindexed optional.
//
// To get an unset optional, just use the zero of Optional[T, I].
func NewUnindexedOptional[T Elementary](val T) Optional[T, Unindexed] {
	return Optional[T, Unindexed]{isSet: true, val: val}
}

// IsSet returns true if the value is set.
func (o Optional[T, I]) IsSet() bool {
	return o.isSet
}

// Get returns the value stored inside or a zero T if the value is unset.
func (o Optional[T, I]) Get() T {
	return o.val
}

// Set stores the value and marks the optional as set.
func (o *Optional[T, I]) Set(val T) {
	o.isSet = true
	o.val = val
}

// Unset flips the optional into the unset state and clears the value to zero.
func (o *Optional[T, I]) Unset() {
	var zero T
	o.isSet = false
	o.val = zero
}

// AsIndexed returns an indexed copy of this property.
func (o *Optional[T, I]) AsIndexed() Optional[T, Indexed] {
	return Optional[T, Indexed]{
		isSet: o.isSet,
		val:   o.val,
	}
}

// AsUnindexed returns an unindexed copy of this property.
func (o *Optional[T, I]) AsUnindexed() Optional[T, Unindexed] {
	return Optional[T, Unindexed]{
		isSet: o.isSet,
		val:   o.val,
	}
}

// FromProperty implements PropertyConverter.
func (o *Optional[T, I]) FromProperty(prop Property) error {
	if prop.Type() == PTNull {
		o.Unset()
		return nil
	}
	var val T
	if err := loadElementary(&val, prop); err != nil {
		return err
	}
	o.isSet = true
	o.val = val
	return nil
}

// ToProperty implements PropertyConverter.
func (o *Optional[T, I]) ToProperty() (prop Property, err error) {
	if !o.isSet {
		return Property{}, ErrSkipProperty
	}
	var idx I
	err = prop.SetValue(o.val, idx.shouldIndex())
	return
}

// Nullable is almost the same as Optional, except absent properties are stored
// as PTNull (instead of being skipped), which means absence of a property can
// be filtered on in queries.
//
// Note that unindexed nullables are represented by unindexed PTNull in the
// datastore. APIs that work on a PropertyMap level can distinguish such
// properties from unindexed optionals (they will see the key in the property
// map). But when using the default PropertyLoadSaver, unindexed nullables and
// unindexed optionals are indistinguishable.
type Nullable[T Elementary, I Indexing] struct {
	isSet bool
	val   T
}

var _ PropertyConverter = &Nullable[bool, Indexed]{}

// NewIndexedNullable creates a new, already set, indexed nullable.
//
// To get an unset nullable, just use the zero of Nullable[T, I].
func NewIndexedNullable[T Elementary](val T) Nullable[T, Indexed] {
	return Nullable[T, Indexed]{isSet: true, val: val}
}

// NewUnindexedNullable creates a new, already set, unindexed nullable.
//
// To get an unset nullable, just use the zero of Nullable[T, I].
func NewUnindexedNullable[T Elementary](val T) Nullable[T, Unindexed] {
	return Nullable[T, Unindexed]{isSet: true, val: val}
}

// IsSet returns true if the value is set.
func (o Nullable[T, I]) IsSet() bool {
	return o.isSet
}

// Get returns the value stored inside or a zero T if the value is unset.
func (o Nullable[T, I]) Get() T {
	return o.val
}

// Set stores the value and marks the nullable as set.
func (o *Nullable[T, I]) Set(val T) {
	o.isSet = true
	o.val = val
}

// Unset flips the nullable into the unset state and clears the value to zero.
func (o *Nullable[T, I]) Unset() {
	var zero T
	o.isSet = false
	o.val = zero
}

// AsIndexed returns an indexed copy of this property.
func (o *Nullable[T, I]) AsIndexed() Nullable[T, Indexed] {
	return Nullable[T, Indexed]{
		isSet: o.isSet,
		val:   o.val,
	}
}

// AsUnindexed returns an unindexed copy of this property.
func (o *Nullable[T, I]) AsUnindexed() Nullable[T, Unindexed] {
	return Nullable[T, Unindexed]{
		isSet: o.isSet,
		val:   o.val,
	}
}

// FromProperty implements PropertyConverter.
func (o *Nullable[T, I]) FromProperty(prop Property) error {
	if prop.Type() == PTNull {
		o.Unset()
		return nil
	}
	var val T
	if err := loadElementary(&val, prop); err != nil {
		return err
	}
	o.isSet = true
	o.val = val
	return nil
}

// ToProperty implements PropertyConverter.
func (o *Nullable[T, I]) ToProperty() (prop Property, err error) {
	var idx I
	if o.isSet {
		err = prop.SetValue(o.val, idx.shouldIndex())
	} else {
		err = prop.SetValue(nil, idx.shouldIndex())
	}
	return
}

// loadElementary is common implementation of FromProperty for optionals and
// nullables.
func loadElementary[T Elementary](val *T, prop Property) error {
	loader := elementaryLoader(reflect.ValueOf(val).Elem())
	if loader.project == PTNull {
		panic("impossible per the generic constraints")
	}
	pVal, err := prop.Project(loader.project)
	if err != nil {
		return err
	}
	if loader.overflow != nil && loader.overflow(pVal) {
		return fmt.Errorf("value %v overflows type %T", pVal, val)
	}
	loader.set(pVal)
	return nil
}
