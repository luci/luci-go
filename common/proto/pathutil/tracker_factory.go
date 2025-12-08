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

package pathutil

import (
	"sync"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protopath"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// $msg key will contain a protopath.RootStep in order to implement Stringer
// nicely.
type messageFieldSteps map[literalField]protopath.Step

func (s messageFieldSteps) String() string {
	return s["$msg"].String()
}

// TrackerFactory allows you to make new Trackers (one per message traversal).
//
// This is meant to be used as a global variable in your module.
//
// This struct will self-initialize on the first call to
// [TrackerFactory[M].New] and cannot be misconfigured.
//
// Example:
//
//	var myTrackerFactory = pathutil.TrackerFactory[*MyMessage] {
//	  AverageRecursiveMessageDepth: 3,
//	}
//
//	func MyFunction() {
//	  t := myTrackerFactory.New(10)
//	}
type TrackerFactory[M proto.Message] struct {
	// once will initialize deepestPath and cachedSteps with all types reachable
	// from `M`.
	//
	// If we include support for Any, we probably should have a secondary cache
	// which can dynamically grow.
	once sync.Once

	// deepestPath is the length of the longest Path needed to address any field
	// transitively reachable from M, excluding recursive fields.
	//
	// Note that Path has the form:
	//
	//   Root(M) / step / step / step
	//
	// So deepestPath needs one extra slot for the root. Additionally, ListIndex/
	// MapIndex are steps in addition to the field they are indexing (that is,
	// `.field[0]` is 2 steps.)
	deepestPath int
	cachedSteps map[protoreflect.MessageDescriptor]messageFieldSteps

	// AverageRecursiveMessageDepth should be set to the expected average depth
	// of any recursive fields in `M`.
	//
	// If `M` does not contain any recursive fields, this can be left as zero.
	//
	// When calling [TrackerFactory[M].New], it will pre-allocate a Path which
	// is as long as the deepest field reachable (non recursively) in `M`, plus
	// this hint.
	//
	// Only used in [TrackerFactory[M].New] - synchronize all modification of
	// this to invocations of New, and you'll be fine (e.g. set it once in init()
	// and then leave it alone).
	AverageRecursiveMessageDepth int
}

func (tf *TrackerFactory[M]) getCachedSteps() map[protoreflect.MessageDescriptor]messageFieldSteps {
	tf.once.Do(func() {
		var initMD func(md protoreflect.MessageDescriptor) int
		initMD = func(md protoreflect.MessageDescriptor) int {
			if tf.cachedSteps == nil {
				tf.cachedSteps = map[protoreflect.MessageDescriptor]messageFieldSteps{}
			}

			fields := md.Fields()
			fieldsLen := fields.Len()

			ret := make(messageFieldSteps, fieldsLen+1)
			ret["$msg"] = protopath.Root(md)

			// All messages have a depth of 1, unless they contain a repeated Message
			// or map field, in which case they have a depth of 2.
			depth := 1
			var toRecurse []protoreflect.MessageDescriptor
			for i := range fieldsLen {
				field := fields.Get(i)
				ret[literalField(field.Name())] = protopath.FieldAccess(field)
				if recurseMsg := field.Message(); recurseMsg != nil {
					toRecurse = append(toRecurse, recurseMsg)
					if field.Cardinality() == protoreflect.Repeated {
						depth = 2
					}
				}
			}
			// Setting this before recursing that we won'tf recurse into ourselves.
			tf.cachedSteps[md] = ret

			for _, msg := range toRecurse {
				// NOTE: If `msg` is recursive, it effectively has a depth of 0 here
				// because we pre-populate tf.cachedSteps to avoid recursively
				// processing The user will supply AverageRecursiveMessageDepth to
				// account for it later.
				if _, ok := tf.cachedSteps[msg]; !ok {
					depth = max(depth, initMD(msg))
				}
			}

			return depth
		}

		var zero M
		tf.deepestPath = initMD(zero.ProtoReflect().Descriptor())
	})
	return tf.cachedSteps
}

// Makes a new Tracker starting at (M) and using the cache in `tf`
// for field steps.
//
// You can use the methods on this Tracker to record errors and step into
// fields in messages of type (M).
func (tf *TrackerFactory[M]) New(maxDepth int) *Tracker {
	cachedSteps := tf.getCachedSteps()

	depth := min(
		tf.deepestPath+tf.AverageRecursiveMessageDepth,
		maxDepth,
	)

	var zero M
	return &Tracker{
		cachedSteps: cachedSteps,
		maxDepth:    maxDepth,
		path:        append(make(protopath.Path, 0, depth), protopath.Root(zero.ProtoReflect().Descriptor())),
		stack:       []messageFieldSteps{cachedSteps[zero.ProtoReflect().Descriptor()]},
	}
}
