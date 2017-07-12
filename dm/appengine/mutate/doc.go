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

// Package mutate includes the main logic of DM's state machine. The package
// is a series of "github.com/luci/luci-go/tumble".Mutation implementations.
// Each mutation operates on a single entity group in DM's datastore model,
// advancing the state machine for the dependency graph by one edge.
package mutate
