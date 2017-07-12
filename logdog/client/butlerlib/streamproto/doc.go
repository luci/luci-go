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

// Package streamproto describes the protocol primitives used by LogDog/Butler
// for stream negotiation.
//
// A LogDog Butler client wishing to create a new LogDog stream can use the
// Flags type to configure/send the stream.
//
// Internally, LogDog represents the Flags properties with the Properties type.
package streamproto
