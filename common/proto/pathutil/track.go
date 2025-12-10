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
	"iter"

	"google.golang.org/protobuf/proto"
)

// TrackOptionalMsg is a helper for [Tracker.Field] for message types to make
// the message access and field name closer in your code, and to help make
// scope issues easier (e.g. being able to use return to exit the outer
// function):
//
//	// current path is `(MyMessage).a`
//	for msg := range protoutil.TrackOptionalMsg(t, "deep_message", parent.GetDeepMessage()) {
//	  // current path is `(MyMessage).a.deep_message`
//	}
//	// Path is `(MyMessage).a` again
//
// This is a no-op if `msg` is nil.
//
// NOTE: The use of `range` here IS strange, but is a quirk of Go. Ideally Go
// would have a `with` keyword to allow the use a coroutine for a singular
// value. Despite this, being able to `return` directly from the loop body
// is very useful.
//
// The full equivalent logic to return an error to the outer function is:
//
//	 for msg := range pathutil.TrackOptionalMsg(t, "deep_message", parent.GetDeepMessage()) {
//	   if err := processDeepMessage(t, parent.GetDeepMessage()); err != nil {
//			 return err
//	   }
//	 }
//
// Instead of:
//
//	 if msg != nil {
//	   var err error
//			t.Field(field, func() {
//			  err = processDeepMessage(t, parent.GetDeepMessage())
//			})
//			if err != nil {
//			  return err
//			}
//	 }
//
// If `range` is too strange for you, just use [Tracker.Field] directly and
// pretend this helper does not exist :).
func TrackOptionalMsg[M interface {
	comparable
	proto.Message
}](t *Tracker, field literalField, msg M) iter.Seq[M] {
	var zero M
	if msg == zero {
		return func(yield func(M) bool) {}
	}
	return func(yield func(M) bool) {
		t.Field(field, func() { yield(msg) })
	}
}

// TrackRequiredMsg is a helper for [Tracker.Field] for message types.
//
// This behaves exactly the same as [TrackOptionalMsg], except if the message
// is nil, this records an error "required".
func TrackRequiredMsg[M interface {
	comparable
	proto.Message
}](t *Tracker, field literalField, msg M) iter.Seq[M] {
	var zero M
	if msg == zero {
		return func(yield func(M) bool) {
			t.FieldErr(field, "required")
		}
	}
	return func(yield func(M) bool) {
		t.Field(field, func() { yield(msg) })
	}
}

// TrackMap is a helper for [Tracker.MapIndex] to make the map access and
// field name closer in your code, and to help make scope issues easier (e.g.
// being able to use break, return, continue):
//
//	// current path is `(MyMessage).a`
//	for key, value := range protoutil.TrackMap(t, "deep_map", parent.GetDeepMap()) {
//	  // current path is `(MyMessage).a.deep_map[key]`
//	}
//	// Path is `(MyMessage).a` again
//
// Instead of:
//
//	// current path is `(MyMessage).a`
//	for key, value := range parent.GetDeepMap() {
//	  t.MapIndex("deep_map", key, func() {
//	    // current path is `(MyMessage).a.deep_map[key]`
//
//	    // However `return` acts like `continue` and `continue` and `break`
//	    // will not compile.
//	  })
//	}
func TrackMap[K comparable, V any](t *Tracker, field literalField, m map[K]V) iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for key, val := range m {
			var stop bool
			t.MapIndex(field, key, func() {
				if !yield(key, val) {
					stop = true
				}
			})
			if stop {
				return
			}
		}
	}
}

// TrackList is a helper for [Tracker.ListIndex] to make the list access and
// field name closer in your code, and to help make scope issues easier (e.g.
// being able to use break, return, continue):
//
//	// current path is `(MyMessage).a`
//	for idx, value := range protoutil.TrackList(t, "deep_list", parent.GetDeepList()) {
//	  // current path is `(MyMessage).a.deep_list[idx]`
//	}
//	// Path is `(MyMessage).a` again
//
// Instead of:
//
//	// current path is `(MyMessage).a`
//	for idx, value := range parent.GetDeepList() {
//	  t.MapIndex("deep_list", key, func() {
//	    // current path is `(MyMessage).a.deep_map[idx]`
//
//	    // However `return` acts like `continue` and `continue` and `break`
//	    // will not compile.
//	  })
//	}
func TrackList[V any](t *Tracker, field literalField, list []V) iter.Seq2[int, V] {
	return func(yield func(int, V) bool) {
		for idx, val := range list {
			var stop bool
			t.ListIndex(field, idx, func() {
				if !yield(idx, val) {
					stop = true
				}
			})
			if stop {
				return
			}
		}
	}
}
