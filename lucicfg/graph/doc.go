// Copyright 2018 The LUCI Authors.
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

// Package graph implements a DAG used internally to represent config objects.
//
// All entities in the LUCI config are represented by nodes in a graph. Nodes
// are linked with directional edges. Cycles are forbidden. Each edge is
// annotated with a name of the relation that added it (e.g. "triggered_by").
// There can be multiple edges between a given pair of nodes, e.g.
// "triggered_by" and "triggers" edges between a builder(...) and a
// trigger(...). When detecting cycles or traversing the graph, edge annotations
// are not considered. They are used only to improve error messages when
// reporting dangling edges (e.g. Builder X references undefined trigger T via
// "triggered_by").
//
// Each node:
//   * Has a unique identifier, called key. A key is a list of
//     (string kind, string id) pairs. Once constructed, a key is an opaque
//     label (e.g. there's no way to explore what's inside the key through
//     Starlark, to keep the API simple). Examples of keys:
//        [core.Bucket("ci")]
//        [core.Bucket("ci"), core.Builder("infra-builder")]
//   * Has a props dict of arbitrary properties (all keys are strings, values
//     are arbitrary).
//   * Has a captured stack trace of where in Starlark it was defined (for error
//     messages).
//   * Is immutable
//
// Structs exposed by this package are not safe for concurrent use without
// external locking.
package graph
