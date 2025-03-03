// Copyright 2025 The LUCI Authors.
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

package errtag

type wrappedErr[T comparable] struct {
	key   TagKey
	inner error
	merge MergeFn[T]
	value T
}

// Error implements the `error` interface and simply returns the string form of
// the inner error.
func (w *wrappedErr[T]) Error() string { return w.inner.Error() }

// Unwrap implements the errors.Unwrap interface and returns the inner error.
func (w *wrappedErr[T]) Unwrap() error { return w.inner }

type wrappedErrIface interface {
	tagValuePtr() (TagKey, any, func(valuePtrs []any) any)
}

var _ wrappedErrIface = (*wrappedErr[bool])(nil)

func (w *wrappedErr[T]) tagValuePtr() (TagKey, any, func(valuePtrs []any) any) {
	return w.key, &w.value, w.mergePtrs
}

func (w *wrappedErr[T]) mergePtrs(valuePtrs []any) any {
	arg := make([]*T, len(valuePtrs))
	for i, v := range valuePtrs {
		arg[i] = v.(*T)
	}
	return w.merge(arg)
}
