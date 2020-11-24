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

// Package git implements derivation of a file graph from git log.
//
// Distance
//
// The distance is derived from the probability that two files appeared in the
// same commit. The core idea is that relevant files tend to be
// modified together.
//
//  Distance(x, y) = -log2(P(y is relevant to x))
//  P(y is relevant to x) := sum(1/(len(c.files)-1) for c in x.commits if y in c.files) / len(x.commits)
//
// or in English, distance from x to y is -log2 of the probability that y
// appears in a commit that touches x and is chosen randomly and independently.
// There are more nuances to this formula, e.g. file removals are not counted
// towards len(commit.files), and commits with len(files) = 1 or
// len(files) > limit are ignored.
//
// Note that this formula penalizes large commits. The more files are modified
// in a commit, the weaker is the strength of its signal.
//
// This graph defines distance only between files, and not directories.
package git
