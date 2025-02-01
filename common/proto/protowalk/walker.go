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

package protowalk

import (
	"google.golang.org/protobuf/proto"
)

// Walker is the type-safe interface for protowalk.
//
// Construct one with [NewWalker].
//
// A zero-value Walker will always return nil Results.
type Walker[M proto.Message] struct {
	dw *DynamicWalker
}

// NewWalker returns a Walker[M] which can be executed against protos of type
// M and process them with the given `processors`.
//
// Mutating `processors` after passing them in here is undefined - don't do it.
func NewWalker[M proto.Message](processors ...FieldProcessor) Walker[M] {
	if len(processors) == 0 {
		return Walker[M]{}
	}
	return Walker[M]{
		NewDynamicWalker((*new(M)).ProtoReflect().Descriptor(), processors...),
	}
}

// Execute evaluates `msg` in terms of all [FieldProcessor]s given when the
// Walker was constructed with NewWalker.
//
// This function is goroutine safe.
//
// This will always return Results whose length is the number of FieldProcessors
// passed to NewWalker. Use [Results.Empty] to check if this contains any
// actionable data. It is not valid to mutate the return value (use
// [Results.Clone] if you need to do this).
func (l Walker[M]) Execute(msg M) Results {
	// This cannot panic - we know that `M` matches the type of the contained
	// DynamicWalker.
	return l.dw.MustExecute(msg)
}
