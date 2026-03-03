// Copyright 2026 The LUCI Authors.
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

package value

import (
	"fmt"

	"google.golang.org/protobuf/proto"
)

// TypePrefix is the constant prefix for all TypeURLs used by TurboCI (via the
// standard `Any` proto message).
const TypePrefix = "type.googleapis.com/"

// URLPatternPackageOf returns a wildcard glob for all proto messages in the
// same package as `T` suitable for use with the TurboCI [TypeSet] type.
//
// For example, if `T` is `my.proto.package.Message`, then this will return a
// pattern matching all types in `my.proto.package`.
func URLPatternPackageOf[T proto.Message]() string {
	var zero T
	return URLPatternPackageOfMsg(zero)
}

// URLPatternPackageOfMsg returns a wildcard glob for all proto messages in the
// same package as `T` suitable for use with the TurboCI [TypeSet] type.
//
// For example, if `T` is `my.proto.package.Message`, then this will return a
// pattern matching all types in `my.proto.package`.
//
// It is allowed for `msg` to be a nil pointer (e.g. `(*MyMessage)(nil)`).
func URLPatternPackageOfMsg(msg proto.Message) string {
	return fmt.Sprintf("%s%s.*",
		TypePrefix, msg.ProtoReflect().Descriptor().ParentFile().Package())
}

// URL returns the full proto `type_url` for a given message type.
func URL[T proto.Message]() string {
	var msg T
	return URLMsg(msg)
}

// URLMsg returns the full proto `type_url` for a given message.
//
// It's allowed for `msg` to be a nil pointer (e.g. `(*MyMessage)(nil)`).
func URLMsg(msg proto.Message) string {
	return fmt.Sprintf("%s%s", TypePrefix, msg.ProtoReflect().Descriptor().FullName())
}
