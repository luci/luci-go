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

// Package filegraph implements a directed graph of files, where weight of the
// edge (A, B) representing how much B is relevant to A. Such graph provides
// relevance between any two files, and can order files by relevance from/to a
// given file.
//
// Relevance and distance
//
// The edge weight can represented via two metrics:
//   - relevance: a value from 0.0 to 1.0, where 0.0 means not relevant
//     at all and 1.0 means extremely relevant.
//   - distance: a value from 0.0 to +inf, where 0.0 means very relevant
//     and +inf means not relevant at all.
//
// One metric can be computed from another:
//
//   Distance := -log(Relevance)
//   Relevance = 2 ** (-Distance)
//
// We can choose the metric to use depending on which one makes it easier
// reason about things in a given context. For example, relevance can be
// used when deriving edge weight from probabilities, and distance can be used
// when applying Dijkstra's algorithm to find relevant files.
// Humans might prefer distance because addition is easier to do mentally
// than multiplication.
package filegraph
