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
)

// RegisterIn returns a RegisteredPropertyIn[InT].
//
// Refer to package documentation for the acceptable types for T.
func RegisterIn[InT any](r *Registry, namespace string, opts ...RegisterOption) (ret RegisteredPropertyIn[InT], err error) {
	o := loadRegOpts(opts)

	var reg registration
	if err = registerInImpl(o, reflect.TypeFor[InT](), namespace, r, &reg, &ret.registeredProperty); err != nil {
		return
	}

	err = r.register(o, namespace, reg)
	return
}

// RegisterOut returns a RegisteredPropertyOut[OutT].
//
// Refer to package documentation for the acceptable types for T.
func RegisterOut[OutT any](r *Registry, namespace string, opts ...RegisterOption) (ret RegisteredPropertyOut[OutT], err error) {
	o := loadRegOpts(opts)

	var reg registration
	if err = registerOutImpl(o, reflect.TypeFor[OutT](), namespace, r, &reg, &ret.registeredProperty); err != nil {
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
	o := loadRegOpts(opts)

	var reg registration
	if err = registerInImpl(o, reflect.TypeFor[InT](), namespace, r, &reg, &ret.RegisteredPropertyIn.registeredProperty); err != nil {
		return
	}
	if err = registerOutImpl(o, reflect.TypeFor[OutT](), namespace, r, &reg, &ret.RegisteredPropertyOut.registeredProperty); err != nil {
		return
	}

	err = r.register(o, namespace, reg)
	return
}

// Register returns a RegisteredProperty with both In and Out types being T.
//
// Refer to package documentation for the acceptable types for T.
func Register[T any](r *Registry, namespace string, opts ...RegisterOption) (ret RegisteredProperty[T, T], err error) {
	o := loadRegOpts(opts)

	var reg registration
	if err = registerInImpl(o, reflect.TypeFor[T](), namespace, r, &reg, &ret.RegisteredPropertyIn.registeredProperty); err != nil {
		return
	}
	if err = registerOutImpl(o, reflect.TypeFor[T](), namespace, r, &reg, &ret.RegisteredPropertyOut.registeredProperty); err != nil {
		return
	}

	err = r.register(o, namespace, reg)
	return
}
