// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package typed is a strictly typed wrapper around cmp.Diff.
package typed

import (
	"github.com/google/go-cmp/cmp"

	"go.chromium.org/luci/common/testing/registry"
)

// Diff is just like cmp.Diff but it forces got and want to have the same type
// and includes protocmp.Transform().
//
// This will result in more informative compile-time errors.
// protocmp.Transform() is necessary for any protocol buffer comparison, and it
// correctly does nothing if the arguments that we pass in are hereditarily
// non-protobufs. So, for developer convenience, let's just always add it.
//
// if you want to extend the defaults, take a look at:
// - "go.chromium.org/luci/common/testing/registry"
func Diff[T any](want T, got T, opts ...cmp.Option) string {
	opts = append(opts, registry.GetCmpOptions()...)
	return cmp.Diff(want, got, opts...)
}

// Got supports the got-before-want style, it can be used as:
//
//	if diff := Got(1).Want(2).Options(...).Diff(); diff != "" {
//	  ...
//	}
func Got[T any](got T) *diffBuilder[T] {
	return &diffBuilder[T]{
		got: got,
	}
}

// diffBuilder builds a diff.
type diffBuilder[T any] struct {
	got     T
	want    T
	options []cmp.Option
}

// Want supplies the want argument to a builder.
func (builder *diffBuilder[T]) Want(want T) *diffBuilder[T] {
	builder.want = want
	return builder
}

// Options supplies the optiosn argument to a builder
func (builder *diffBuilder[T]) Options(options ...cmp.Option) *diffBuilder[T] {
	builder.options = options
	return builder
}

// Diff produces a diff from a diffbuilder.
func (builder *diffBuilder[T]) Diff() string {
	return Diff(builder.want, builder.got, builder.options...)
}
