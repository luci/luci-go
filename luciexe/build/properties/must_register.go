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

// MustRegisterIn is a slimmed version of RegisterIn, to be used at init()-time
// where you statically know that there should never be a registration error.
func MustRegisterIn[InT any](r *Registry, namespace string, opts ...RegisterOption) RegisteredPropertyIn[InT] {
	ret, err := RegisterIn[InT](r, namespace, append([]RegisterOption{OptSkipFrames(1)}, opts...)...)
	if err != nil {
		panic(err)
	}
	return ret
}

// MustRegister is a slimmed version of Register, to be used at init()-time
// where you statically know that there should never be a registration error.
func MustRegister[T any](r *Registry, namespace string, opts ...RegisterOption) RegisteredProperty[T, T] {
	ret, err := Register[T](r, namespace, append([]RegisterOption{OptSkipFrames(1)}, opts...)...)
	if err != nil {
		panic(err)
	}
	return ret
}

// MustRegisterOut is a slimmed version of RegisterOut, to be used at init()-time
// where you statically know that there should never be a registration error.
func MustRegisterOut[OutT any](r *Registry, namespace string, opts ...RegisterOption) RegisteredPropertyOut[OutT] {
	ret, err := RegisterOut[OutT](r, namespace, append([]RegisterOption{OptSkipFrames(1)}, opts...)...)
	if err != nil {
		panic(err)
	}
	return ret
}

// MustRegisterInOut is a slimmed version of RegisterInOut, to be used at init()-time
// where you statically know that there should never be a registration error.
func MustRegisterInOut[InT, OutT any](r *Registry, namespace string, opts ...RegisterOption) RegisteredProperty[InT, OutT] {
	ret, err := RegisterInOut[InT, OutT](r, namespace, append([]RegisterOption{OptSkipFrames(1)}, opts...)...)
	if err != nil {
		panic(err)
	}
	return ret
}
