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

// Package filegraph implements a graph of files in a git repository, where a
// node is a file and a weight from 0.0 to 1.0 of edge (A, B) represents how
// much file B is relevant to file A. The tool has subcommands to compute
// relevance between any two files based on the shortest path, where distance is
// -log(relevance), as well as order files by distance from a given set of root
// files. This can be used to order tests by relevance to files modified in a
// patchset.
//
// TODO(nodir): elaborate.
package filegraph
