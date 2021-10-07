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

package jobexport

// Running `go generate` will regenerate the '.job.json' and '.swarm.json'
// files in the 'testdata' directory.
//
// The .job.json files are the output from the `jobcreate` test; they are
// job.Definition JSONPB messages.
//
// When doing this make sure to inspect the diff of the .swarm.json files to
// ensure they're correct.

//go:generate cp ../jobcreate/testdata/bbagent_cas.job.json testdata/
//go:generate cp ../jobcreate/testdata/raw_cas.job.json testdata/
//go:generate go test -train
