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
	"go.chromium.org/luci/common/errors"
)

// Uploader provides the API to upload the files in an isolate file to CAS service.
type Uploader struct {
	cas     *client.Client
	fmCache filemetadata.Cache
	deduper *ChunkerDeduper
}

// UploaderOption is the type to configure an Uploader object.
type UploaderOption func(*Uploader)

// WithCache configures the file metadata cache an uploader uses.
// This exists mostly for testing purpose.
func WithCache(c filemetadata.Cache) UploaderOption {
	return func(up *Uploader) {
		up.fmCache = c
	}
}

// NewUploader creates a new Uploader object.
func NewUploader(cas *client.Client, opts ...UploaderOption) *Uploader {
	up := &Uploader{
		cas:     cas,
		deduper: NewChunkerDeduper(),
	}
	for _, o := range opts {
		o(up)
	}
	if up.fmCache == nil {
		up.fmCache = filemetadata.NewSingleFlightCache()
	}
	return up
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
		inputSpec.InputExclusions = []*command.InputExclusion{
			&command.InputExclusion{
				Regex: opts.IgnoredPathFilterRe,
				Type:  command.UnspecifiedInputType,
			},
		}
	}

	return execRoot, inputSpec, nil
}

// Upload parses the isolate options and uploads the files they refer to to
// CAS. Returns the list of digests  of the root directories. The returned
// digests will have the same order as |opts|.
func (up *Uploader) Upload(ctx context.Context, opts ...*isolate.ArchiveOptions) ([]digest.Digest, error) {
	var chunkers []*chunker.Chunker
	var rootDgs []digest.Digest

	for _, opt := range opts {
		execRoot, inputSpec, err := buildInputSpec(opt)
		if err != nil {
			return nil, err
		}
		rootDg, chks, _, err := tree.ComputeMerkleTree(execRoot, inputSpec, chunker.DefaultChunkSize, up.fmCache)
		if err != nil {
			return nil, errors.Annotate(err, "failed to call ComputeMerkleTree() exeRoot=%s inputSpec=%+v", execRoot, inputSpec).Err()
		}
		rootDgs = append(rootDgs, rootDg)
		chunkers = append(chunkers, up.deduper.Deduplicate(chks)...)
	}
	// TODO: handle the stats
	if _, err := up.cas.UploadIfMissing(ctx, chunkers...); err != nil {
		return nil, errors.Annotate(err, "failed to upload to CAS").Err()
	}
	return rootDgs, nil
}
