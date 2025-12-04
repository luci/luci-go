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
	"google.golang.org/protobuf/reflect/protopath"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/common/errors"
)

// DynamicWalker is a non-generic version of Walker.
//
// This is useful when you need to generate Walker instances dynamically. If you
// statically know the message types you're handling, use Walker instead.
//
// Construct with [NewDynamicWalker].
//
// A zero-value DynamicWalker will always return nil Results.
type DynamicWalker struct {
	msgD     protoreflect.MessageDescriptor
	procs    []FieldProcessor
	plans    map[protoreflect.MessageDescriptor]plan
	emptyRet Results
}

// NewDynamicWalker constructs a new [DynamicWalker].
func NewDynamicWalker(msgD protoreflect.MessageDescriptor, processors ...FieldProcessor) *DynamicWalker {
	if len(processors) == 0 {
		return nil
	}

	tCache := tempCache{}
	tCache.populate(msgD, processors)

	ret := &DynamicWalker{
		msgD:  msgD,
		procs: processors,
	}
	if len(tCache) > 0 {
		ret.plans = make(map[protoreflect.MessageDescriptor]plan, len(tCache))
		for desc := range tCache {
			ret.plans[desc] = makePlan(desc, processors, tCache)
		}
	} else {
		ret.emptyRet = make(Results, len(processors))
	}
	return ret
}

// Execute evaluates `msg` in terms of all [FieldProcessor]s given when the
// DynamicWalker was constructed with NewWalker.
//
// This will only return an error if the type of `msg` does not match the type
// that was given to NewDynamicWalker. If you statically know that `msg` is of
// the same type, use MustExecute.
//
// This function is goroutine safe.
//
// This will always return Results whose length is the number of FieldProcessors
// passed to NewDynamicWalker. Use [Results.Empty] to check if this contains
// any actionable data. It is not valid to mutate the return value (use
// [Results.Clone] if you need to do this).
func (l *DynamicWalker) Execute(msg proto.Message) (Results, error) {
	if l == nil {
		return nil, nil
	}
	if l.emptyRet != nil {
		return l.emptyRet, nil
	}
	ret := make(Results, len(l.procs))
	msgD := msg.ProtoReflect().Descriptor()
	if msg.ProtoReflect().Descriptor() != l.msgD {
		return nil, errors.Fmt("mismatched message types, got %s, expected %s", msgD.FullName(), l.msgD.FullName())
	}

	// 32 is a guess at how deep the Path could get.
	//
	// There's really no way to know ahead of time (since proto messages could
	// have a recursive structure, allowing the expression of trees, etc.)
	mappedRet := l.fieldsImpl(make(protopath.Path, 0, 32), msg.ProtoReflect())
	for i, proc := range l.procs {
		ret[i] = mappedRet[proc]
	}
	return ret, nil
}

// MustExecute is the same as Execute, except it will panic if the type of `msg`
// does not match the type that was given to NewDynamicWalker.
//
// If you do not statically know that `msg` is of the same type, use Execute and
// handle the error.
func (l *DynamicWalker) MustExecute(msg proto.Message) Results {
	ret, err := l.Execute(msg)
	if err != nil {
		panic(err)
	}
	return ret
}
