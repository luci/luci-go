// Copyright 2024 The LUCI Authors.
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

package properties

import (
	"reflect"

	"go.chromium.org/luci/common/errors"
)

// RegisterIn returns a RegisteredPropertyIn[InT].
//
// Refer to package documentation for the acceptable types for T.
func RegisterIn[InT any](r *Registry, namespace string, opts ...RegisterOption) (ret RegisteredPropertyIn[InT], err error) {
	defer func() {
		if err != nil {
			err = errors.Annotate(err, "RegisterIn[%T]", *new(InT)).Err()
		}
	}()
	inTyp := reflect.TypeFor[InT]()

	o, err := loadRegOpts(namespace, opts, inTyp, nil)
	if err != nil {
		return
	}

	var reg registration
	if err = registerInImpl(o, inTyp, namespace, r, &reg, &ret.registeredProperty); err != nil {
		return
	}

	err = r.register(o, namespace, reg)
	return
}

// RegisterOut returns a RegisteredPropertyOut[OutT].
//
// Refer to package documentation for the acceptable types for T.
func RegisterOut[OutT any](r *Registry, namespace string, opts ...RegisterOption) (ret RegisteredPropertyOut[OutT], err error) {
	defer func() {
		if err != nil {
			err = errors.Annotate(err, "RegisterOut[%T]", *new(OutT)).Err()
		}
	}()
	outTyp := reflect.TypeFor[OutT]()

	o, err := loadRegOpts(namespace, opts, nil, outTyp)
	if err != nil {
		return
	}

	var reg registration
	if err = registerOutImpl(o, outTyp, namespace, r, &reg, &ret.registeredProperty); err != nil {
		return
	}

	err = r.register(o, namespace, reg)
	return
}

// RegisterInOut returns a RegisteredProperty with differint types for InT and
// OutT. Prefer Register if you need the same input and output types.
//
// Refer to package documentation for the acceptable types for T.
func RegisterInOut[InT, OutT any](r *Registry, namespace string, opts ...RegisterOption) (ret RegisteredProperty[InT, OutT], err error) {
	defer func() {
		if err != nil {
			err = errors.Annotate(err, "RegisterInOut[%T, %T]", *new(InT), *new(OutT)).Err()
		}
	}()
	inTyp, outTyp := reflect.TypeFor[InT](), reflect.TypeFor[OutT]()

	o, err := loadRegOpts(namespace, opts, inTyp, outTyp)
	if err != nil {
		return
	}

	var reg registration
	if err = registerInImpl(o, inTyp, namespace, r, &reg, &ret.RegisteredPropertyIn.registeredProperty); err != nil {
		return
	}
	if err = registerOutImpl(o, outTyp, namespace, r, &reg, &ret.RegisteredPropertyOut.registeredProperty); err != nil {
		return
	}

	err = r.register(o, namespace, reg)
	return
}

// Register returns a RegisteredProperty with both In and Out types being T.
//
// Refer to package documentation for the acceptable types for T.
func Register[T any](r *Registry, namespace string, opts ...RegisterOption) (ret RegisteredProperty[T, T], err error) {
	defer func() {
		if err != nil {
			err = errors.Annotate(err, "Register[%T]", *new(T)).Err()
		}
	}()
	typ := reflect.TypeFor[T]()

	o, err := loadRegOpts(namespace, opts, typ, typ)
	if err != nil {
		return
	}

	var reg registration
	if err = registerInImpl(o, typ, namespace, r, &reg, &ret.RegisteredPropertyIn.registeredProperty); err != nil {
		return
	}
	if err = registerOutImpl(o, typ, namespace, r, &reg, &ret.RegisteredPropertyOut.registeredProperty); err != nil {
		return
	}

	err = r.register(o, namespace, reg)
	return
}
