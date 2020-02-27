// Copyright 2020 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package testdata

// Running `go generate` will regenerate the '.job.json' and '.swarm.json'
// files.
//
// The .job.json files are the output from the `jobcreate` test; they are
// job.Definition JSONPB messages.
//
// When doing this make sure to inspect the diff of the .swarm.json files to
// ensure they're correct.

//go:generate cp ../../jobcreate/testdata/bbagent.job.json .
//go:generate cp ../../jobcreate/testdata/raw.job.json .
//go:generate go test .. -train
