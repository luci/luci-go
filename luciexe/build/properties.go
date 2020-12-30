// Copyright 2020 The LUCI Authors.
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

package build

// MakePropertyReader allows your library/module to reserve a section of the
// input properties for itself.
//
// Attempting to reserve duplicate namespaces will panic. The namespace refers
// to the top-level property key. It is recommended that:
//   * The `ns` begins with '$'.
//   * The value after the '$' is the canonical Go package name for your
//     library.
//
// Using the generated function will parse the relevant input property namespace
// as JSONPB, returning the parsed message (and an error, if any).
//
//   var myPropertyReader func(context.Context) (*MyPropertyMsg, error)
//   func init() {
//     MakePropertyReader("$some/namespace", &myPropertyReader)
//   }
//
// In Go2 this will be less weird:
//   type PropertyReader[T proto.Message] func(context.Context) (T, error)
//   MakePropertyReader[T proto.Message](ns string) PropertyReader[T]
func MakePropertyReader(ns string, fnptr interface{}) {
	panic("implement")
}

// MakePropertyModifier allows your library/module to reserve a section of the
// output properties for itself.
//
// You can use this to obtain a write function (replace contents at namespace)
// and/or a merge function (do proto.Merge on the current contents of that
// namespace). If one of the function pointers is nil, it will be skipped (at
// least one must be non-nil). If both function pointers are provided, their
// types must exactly agree.
//
// Attempting to reserve duplicate namespaces will panic. The namespace refers
// to the top-level property key. It is recommended that:
//   * The `ns` begins with '$'.
//   * The value after the '$' is the canonical Go package name for your
//     library.
//
// You should call this at init()-time like:
//
//   var propWriter func(context.Context, *MyMessage)
//   var propMerger func(context.Context, *MyMessage)
//
//   ... = Start(, ..., OptOutputProperties(&writer, &merger))
//   func init() {
//     // one of the two function pointers may be nil
//     MakePropertyModifier("$some/namespace", &propWriter, &propMerger)
//   }
//
// In Go2 this will be less weird:
//   type PropertyModifier[T proto.Message] interface {
//     Write(context.Context, value T) // assigns 'value'
//     Merge(context.Context, value T) // does proto.Merge(current, value)
//   }
//   func MakePropertyModifier[T proto.Message](ns string) PropertyModifier[T]
func MakePropertyModifier(ns string, writeFnptr, mergeFnptr interface{}) {
	panic("implement")
}
