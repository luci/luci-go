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

package cas

import (
	"context"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/chunker"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/command"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/filemetadata"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/tree"
	"go.chromium.org/luci/client/isolate"
)

// Uploader provides the API to upload the files in an isolate file to CAS service.
type Uploader struct {
	ctx       context.Context
	casClient *client.Client
	chunkSize int
	fmCache   filemetadata.Cache
	deduper   *ChunkerDeduper
}

// NewUploader creates a new Uploader object.
func NewUploader(ctx context.Context, casClient *client.Client) *Uploader {
	return &Uploader{
		ctx:       ctx,
		casClient: casClient,
		chunkSize: chunker.DefaultChunkSize,
		fmCache:   filemetadata.NewSingleFlightCache(),
		deduper:   NewChunkerDeduper(),
	}
}

func buildInputSpec(opts *isolate.ArchiveOptions) (string, *command.InputSpec, error) {
	inputPaths, execRoot, err := isolate.ProcessIsolateForCAS(opts)
	if err != nil {
		return "", nil, err
	}

	inputSpec := &command.InputSpec{
		Inputs: inputPaths,
	}
	if opts.IgnoredPathFilterRe != "" {
		excl := &command.InputExclusion{
			Regex: opts.IgnoredPathFilterRe,
			Type:  command.UnspecifiedInputType,
		}
		inputSpec.InputExclusions = append(inputSpec.InputExclusions, excl)
	}

	return execRoot, inputSpec, nil
}

// Upload parses the isolate options and uploads the files it refers to to CAS. Returns the digest
// of the root directory, if there is no error.
func (up *Uploader) Upload(opts *isolate.ArchiveOptions) (digest.Digest, error) {
	execRoot, inputSpec, err := buildInputSpec(opts)
	if err != nil {
		return digest.Empty, err
	}
	rootDg, chunkers, _, err := tree.ComputeMerkleTree(execRoot, inputSpec, up.chunkSize, up.fmCache)
	if err != nil {
		return digest.Empty, err
	}
	chunkers = up.deduper.Deduplicate(chunkers)
	err = up.casClient.UploadIfMissing(up.ctx, chunkers...)
	if err != nil {
		return digest.Empty, err
	}
	return rootDg, nil
}

// Close closes the uploader object.
func (up *Uploader) Close() error {
	return up.casClient.Close()
}
