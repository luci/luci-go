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

package sink

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	sinkpb "go.chromium.org/luci/resultdb/sink/proto/v1"
)

type uploadTask struct {
	art        *sinkpb.Artifact
	artName    string
	size       int64 // content size
	testStatus pb.TestStatus
}

// newUploadTask constructs an uploadTask for the artifact.
//
// If FilePath is set on the artifact, this calls os.Stat to obtain the file information,
// and may return an error if the Stat call fails. e.g., permission denied, not found.
// It also returns an error if the artifact file path is a directory.
func newUploadTask(name string, art *sinkpb.Artifact, testStatus pb.TestStatus) (*uploadTask, error) {
	ret := &uploadTask{
		art:        art,
		artName:    name,
		size:       int64(len(art.GetContents())),
		testStatus: testStatus,
	}

	// Find and save the content size on uploadTask creation, so that the task scheduling
	// and processing logic can use the size information w/o issuing system calls.
	if fp := art.GetFilePath(); fp != "" {
		st, err := os.Stat(fp)
		switch {
		case err != nil:
			return nil, errors.Fmt("querying file info: %w", err)
		case st.Mode().IsRegular():
			// break

		// Return a more human friendly error than 1000....0.
		case st.IsDir():
			return nil, errors.Fmt("%q is a directory", fp)
		default:
			return nil, errors.Fmt("%q is not a regular file: %s", fp, strconv.FormatInt(int64(st.Mode()), 2))
		}
		ret.size = st.Size()
	}
	return ret, nil
}

// CreateRequest returns a CreateArtifactRequest for the upload task.
//
// Note that this will open and read content from the file, the artifact is set with
// Artifact_FilePath. Save the returned request to avoid unnecessary I/Os,
// if necessary.
func (t *uploadTask) CreateRequest() (*pb.CreateArtifactRequest, error) {
	invID, tID, rID, aID, err := pbutil.ParseArtifactName(t.artName)
	if err != nil {
		return nil, err
	}
	req := &pb.CreateArtifactRequest{
		Artifact: &pb.Artifact{
			ArtifactId:  aID,
			ContentType: t.art.GetContentType(),
			SizeBytes:   t.size,
			Contents:    t.art.GetContents(),
			GcsUri:      t.art.GetGcsUri(),
			TestStatus:  t.testStatus,
		},
	}

	// parent
	switch {
	case tID == "":
		// Invocation-level artifact
		req.Parent = pbutil.InvocationName(invID)
	default:
		req.Parent = pbutil.TestResultName(invID, tID, rID)
	}

	// contents
	if fp := t.art.GetFilePath(); fp != "" {
		if req.Artifact.Contents, err = os.ReadFile(fp); err != nil {
			return nil, err
		}
	}

	// Update the content type if it is missing
	req.Artifact.ContentType = artifactContentType(req.Artifact.ContentType, req.Artifact.Contents)

	// Perform size check only for non gcs artifact.
	if req.Artifact.GcsUri == "" {
		// If the size of the read content is different to what stat claimed initially, then
		// return an error, so that the batching logic can be kept simple. Test frameworks
		// should send finalized artifacts only.
		if int64(len(req.Artifact.Contents)) != t.size {
			return nil, errors.Fmt("the size of the artifact contents changed from %d to %d",
				t.size, len(req.Artifact.Contents))
		}
	}

	return req, nil
}

type artifactChannel struct {
	// batchChannel uploads artifacts via pb.BatchCreateArtifacts().
	//
	// This batches input artifacts and uploads them all at once.
	// This is suitable for uploading a large number of small artifacts.
	//
	// The downside of this channel is that there is a limit on the maximum size of
	// an artifact that can be included in a batch. Use streamChannel for artifacts
	// greater than ServerConfig.MaxBatchableArtifactSize.
	batchChannel dispatcher.Channel[*uploadTask]

	// streamChannel uploads artifacts in a streaming manner via HTTP.
	//
	// This is suitable for uploading large files, but with limited parallelism.
	// Use batchChannel, if possible.
	streamChannel dispatcher.Channel[*uploadTask]

	// wgActive indicates if there are active goroutines invoking reportTestResults.
	//
	// reportTestResults can be invoked by multiple goroutines in parallel. wgActive is used
	// to ensure that all active goroutines finish enqueuing messages to the channel before
	// closeAndDrain closes and drains the channel.
	wgActive sync.WaitGroup

	// 1 indicates that artifactChannel started the process of closing and draining
	// the channel. 0, otherwise.
	closed int32

	cfg *ServerConfig
}

func newArtifactChannel(ctx context.Context, cfg *ServerConfig) *artifactChannel {
	var err error
	c := &artifactChannel{cfg: cfg}
	au := artifactUploader{
		MaxBatchable: cfg.MaxBatchableArtifactSize,
		Recorder:     cfg.Recorder,
		StreamClient: cfg.ArtifactStreamClient,
		StreamHost:   cfg.ArtifactStreamHost,
	}

	// batchChannel
	bcOpts := &dispatcher.Options[*uploadTask]{
		Buffer: buffer.Options{
			// BatchCreateArtifactRequest can include up to 500 requests and at most 10MiB
			// of artifact contents. uploadTaskSlicer slices tasks, as the number of size
			// limits apply.
			//
			// It's recommended to keep BatchItemsMax >= 500 to increase the chance of
			// BatchCreateArtifactRequest to contain 500 artifacts.
			//
			// Depending on the estimated pattern of artifact size distribution, consider
			// to tune ServerConfig.MaxBatchableArtifactSize and BatchDuration to find
			// the optimal point between artifact upload latency and throughput.
			//
			// For more details, visit
			// https://godoc.org/go.chromium.org/luci/resultdb/proto/v1#BatchCreateArtifactsRequest
			BatchItemsMax: 500,
			MaxLeases:     int(cfg.ArtChannelMaxLeases),
			FullBehavior:  &buffer.BlockNewItems{MaxItems: 8000},
		},
	}
	c.batchChannel, err = dispatcher.NewChannel(ctx, bcOpts, func(b *buffer.Batch[*uploadTask]) error {
		return errors.WrapIf(au.BatchUpload(ctx, b), "BatchUpload")
	})
	if err != nil {
		panic(fmt.Sprintf("failed to create batch channel for artifacts: %s", err))
	}

	// streamChannel
	stOpts := &dispatcher.Options[*uploadTask]{
		Buffer: buffer.Options{
			// BatchItemsMax MUST be 1.
			BatchItemsMax: 1,
			MaxLeases:     int(cfg.ArtChannelMaxLeases),
			FullBehavior:  &buffer.BlockNewItems{MaxItems: 4000},
		},
	}
	c.streamChannel, err = dispatcher.NewChannel(ctx, stOpts, func(b *buffer.Batch[*uploadTask]) error {
		return errors.WrapIf(
			au.StreamUpload(ctx, b.Data[0].Item, cfg.UpdateToken),
			"StreamUpload")
	})
	if err != nil {
		panic(fmt.Sprintf("failed to create stream channel for artifacts: %s", err))
	}
	return c
}

func (c *artifactChannel) closeAndDrain(ctx context.Context) {
	// mark the channel as closed, so that schedule() won't accept new tasks.
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return
	}
	// wait for all the active sessions to finish enquing tests results to the channel
	c.wgActive.Wait()

	var draining sync.WaitGroup
	draining.Add(2)
	go func() {
		defer draining.Done()
		c.batchChannel.CloseAndDrain(ctx)
	}()
	go func() {
		defer draining.Done()
		c.streamChannel.CloseAndDrain(ctx)
	}()
	draining.Wait()
}

func (c *artifactChannel) schedule(tasks ...*uploadTask) {
	c.wgActive.Add(1)
	defer c.wgActive.Done()
	// if the channel already has been closed, drop the test results.
	if atomic.LoadInt32(&c.closed) == 1 {
		return
	}

	for _, task := range tasks {
		if task.size > c.cfg.MaxBatchableArtifactSize {
			c.streamChannel.C <- task
		} else {
			c.batchChannel.C <- task
		}
	}
}

// artifactContentType gets the MIME media type by looking at the content.
// It considers at most the first 512 bytes of content.
// If the contentType is already present, it returns that instead.
func artifactContentType(contentType string, contents []byte) string {
	if len(contentType) != 0 || len(contents) == 0 {
		return contentType
	}

	mediaType, _, err := mime.ParseMediaType(http.DetectContentType(contents))
	if err != nil {
		return ""
	}

	return mediaType
}
