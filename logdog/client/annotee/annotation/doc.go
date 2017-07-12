// Copyright 2015 The LUCI Authors.
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

// Package annotation implements a state machine that constructs Milo annotation
// protobufs from a series of annotation commands.
//
// The annotation state is represented by a State object. Annotation strings are
// appended to the State via Append(), causing the State to incorporate that
// annotation and advance. During state advancement, any number of the State's
// Callbacks may be invoked in response to changes that are made.
//
// State is pure (not bound to any I/O). Users of a State should interact with
// it by implementing Callbacks.
package annotation
