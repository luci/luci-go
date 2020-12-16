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

package ledcmd

import (
	"context"
	"crypto"
	"encoding/json"
	"io"
	"io/ioutil"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/led/job"
	swarmingpb "go.chromium.org/luci/swarming/proto/api"
)

type fakePendingItem struct {
	dgst isolated.HexDigest
}

func (f fakePendingItem) WaitForHashed()             {}
func (f fakePendingItem) Digest() isolated.HexDigest { return f.dgst }

// TODO(crbug.com/1066777): This is really quite bad. The fake logic here is
// sufficiently complex to warrant tests, but `isolatedclient` as a package is
// not setup to allow for fakes or mocks (at the time of writing,
// *isolatedclient.Client is a struct).
//
// Interface-ifying the Client object and moving this fake adjacent to
// isolatedclient and then adding tests for it is feasible, but would require
// updating all the callsites of isolatedclient to use the interface, and at the
// time of writing I can't justify the time investment.
type fakeIsolateClient struct {
	m sync.Mutex

	// maps hash -> data
	casMap map[isolated.HexDigest]string

	err errors.MultiError
}

var _ isoClientIface = (*fakeIsolateClient)(nil)

func (f *fakeIsolateClient) Server() string    { return "iso.example.com" }
func (f *fakeIsolateClient) Namespace() string { return "gzip" }
func (f *fakeIsolateClient) Close() error {
	if len(f.err) > 0 {
		return f.err
	}
	return nil
}

func (f *fakeIsolateClient) Fetch(ctx context.Context, dgst isolated.HexDigest, w io.Writer) error {
	f.m.Lock()
	defer f.m.Unlock()
	data, ok := f.casMap[dgst]
	if !ok {
		return errors.New("no such item")
	}
	_, err := io.WriteString(w, data)
	return err
}

func (f *fakeIsolateClient) Push(filename string, data isolatedclient.Source, priority int64) pendingItem {
	dataReader, err := data()
	if err != nil {
		f.err = append(f.err, errors.Reason("failed to get data reader for %q", filename).Err())
		return fakePendingItem{}
	}
	defer dataReader.Close()

	dataRaw, err := ioutil.ReadAll(dataReader)
	if err != nil {
		f.err = append(f.err, errors.Reason("failed to get data for %q", filename).Err())
		return fakePendingItem{}
	}

	dgst := isolated.HashBytes(crypto.SHA1, dataRaw)
	f.m.Lock()
	defer f.m.Unlock()
	f.casMap[dgst] = string(dataRaw)
	return fakePendingItem{dgst}
}

func (f *fakeIsolateClient) pushString(data string) string {
	dgst := isolated.HashBytes(crypto.SHA1, []byte(data))
	f.m.Lock()
	defer f.m.Unlock()
	f.casMap[dgst] = data
	return string(dgst)
}

func (f *fakeIsolateClient) pushFile(data string) isolated.File {
	return isolated.BasicFile(
		isolated.HexDigest(f.pushString(data)), 0555, int64(len(data)))
}

func (f *fakeIsolateClient) tree(dgst string) *swarmingpb.CASTree {
	return &swarmingpb.CASTree{
		Digest:    dgst,
		Namespace: f.Namespace(),
		Server:    f.Server(),
	}
}

func (f *fakeIsolateClient) mkIso(fileContent ...string) *isolated.Isolated {
	if len(fileContent)%2 != 0 {
		panic("mkIso takes (filename, data) pairs")
	}
	ret := isolated.New(isolated.GetHash(f.Namespace()))
	for i := 0; i < len(fileContent); i += 2 {
		name, data := fileContent[i], fileContent[i+1]
		ret.Files[name] = f.pushFile(data)
	}
	return ret
}

func (f *fakeIsolateClient) pushIso(iso *isolated.Isolated) *swarmingpb.CASTree {
	marshalled, err := json.Marshal(iso)
	So(err, ShouldBeNil)
	return f.tree(f.pushString(string(marshalled)))
}

func isoTestContext() (context.Context, *fakeIsolateClient) {
	client := &fakeIsolateClient{casMap: map[isolated.HexDigest]string{}}
	return context.WithValue(context.Background(), &isolateTestIfaceKey, client), client
}

func testSWJob() *job.Definition {
	return &job.Definition{JobType: &job.Definition_Swarming{
		Swarming: &job.Swarming{
			Task: &swarmingpb.TaskRequest{},
		},
	}}
}

func TestConsolidateIsolateSources(t *testing.T) {
	t.Parallel()

	Convey(`ConsolidateIsolateSources`, t, func() {
		ctx, client := isoTestContext()

		Convey(`empty`, func() {
			job := testSWJob()
			So(ConsolidateIsolateSources(ctx, nil, job), ShouldBeNil)
		})

		Convey(`slices without isolates`, func() {
			job := testSWJob()
			sw := job.GetSwarming()
			sw.Task.TaskSlices = append(sw.Task.TaskSlices, &swarmingpb.TaskSlice{
				Properties: &swarmingpb.TaskProperties{},
			})
			sw.Task.TaskSlices = append(sw.Task.TaskSlices, &swarmingpb.TaskSlice{
				Properties: &swarmingpb.TaskProperties{},
			})

			Convey(`without isolates`, func() {
				So(ConsolidateIsolateSources(ctx, nil, job), ShouldBeNil)
				curIso, err := job.Info().CurrentIsolated()
				So(err, ShouldBeNil)
				So(curIso.CASTree, ShouldBeNil)
			})

			Convey(`with UserPayload`, func() {
				job.UserPayload = client.pushIso(client.mkIso(
					"some_file", "I am a banana",
				))

				dgst := job.UserPayload.Digest

				So(ConsolidateIsolateSources(ctx, nil, job), ShouldBeNil)

				So(sw.Task.TaskSlices[0].Properties.CasInputs, ShouldResemble,
					client.tree(dgst))
				So(sw.Task.TaskSlices[1].Properties.CasInputs, ShouldResemble,
					client.tree(dgst))

				curIso, err := job.Info().CurrentIsolated()
				So(err, ShouldBeNil)
				So(curIso.CASTree, ShouldResemble, client.tree(dgst))
			})

			Convey(`with individual isolates`, func() {
				sw.Task.TaskSlices[0].Properties.CasInputs = client.pushIso(client.mkIso(
					"some_file", "I am a banana",
				))
				sw.Task.TaskSlices[1].Properties.CasInputs = client.pushIso(client.mkIso(
					"some_other_file", "I am totally a banana",
				))
				dgst0 := sw.Task.TaskSlices[0].Properties.CasInputs.Digest
				dgst1 := sw.Task.TaskSlices[1].Properties.CasInputs.Digest

				So(ConsolidateIsolateSources(ctx, nil, job), ShouldBeNil)

				So(sw.Task.TaskSlices[0].Properties.CasInputs, ShouldResemble,
					client.tree(dgst0))
				So(sw.Task.TaskSlices[1].Properties.CasInputs, ShouldResemble,
					client.tree(dgst1))

				curIso, err := job.Info().CurrentIsolated()
				So(err, ShouldErrLike, "contains multiple isolate inputs")
				So(curIso, ShouldBeNil)
			})

			Convey(`UserPayload and slices`, func() {
				job.UserPayload = client.pushIso(client.mkIso(
					"user_payload", "file contents",
				))
				sliceTree := client.pushIso(client.mkIso(
					"slice_content", "some stuff",
				))
				sw.Task.TaskSlices[0].Properties.CasInputs = sliceTree
				sw.Task.TaskSlices[1].Properties.CasInputs = sliceTree

				So(ConsolidateIsolateSources(ctx, nil, job), ShouldBeNil)

				curIso, err := job.Info().CurrentIsolated()
				So(err, ShouldBeNil)
				So(curIso.CASTree, ShouldResemble, client.pushIso(client.mkIso(
					"user_payload", "file contents",
					"slice_content", "some stuff",
				)))
			})

			Convey(`can follow includes`, func() {
				job.UserPayload = &swarmingpb.CASTree{
					Server:    client.Server(),
					Namespace: client.Namespace(),
				}
				iso := client.mkIso("some_file", "data")
				subIso := client.pushIso(client.mkIso(
					"other_file", "data",
				))
				iso.Includes = append(iso.Includes, isolated.HexDigest(subIso.Digest))
				isoTree := client.pushIso(iso)
				sw.Task.TaskSlices[0].Properties.CasInputs = isoTree
				sw.Task.TaskSlices[1].Properties.CasInputs = isoTree
				So(ConsolidateIsolateSources(ctx, nil, job), ShouldBeNil)

				So(sw.Task.TaskSlices[0].Properties.CasInputs, ShouldResemble, client.pushIso(client.mkIso(
					"some_file", "data",
					"other_file", "data",
				)))
			})

			Convey(`overlapping files`, func() {
				rootIsoTree := client.pushIso(client.mkIso("diamonds", "forever"))

				leftIso := client.mkIso("left", "shark")
				leftIso.Includes = append(leftIso.Includes, isolated.HexDigest(rootIsoTree.Digest))
				leftIsoTree := client.pushIso(leftIso)

				rightIso := client.mkIso("right", "shark")
				rightIso.Includes = append(rightIso.Includes, isolated.HexDigest(rootIsoTree.Digest))
				rightIsoTree := client.pushIso(rightIso)

				topIso := client.mkIso("tippy", "top")
				topIso.Includes = append(topIso.Includes,
					isolated.HexDigest(leftIsoTree.Digest),
					isolated.HexDigest(rightIsoTree.Digest),
				)
				job.UserPayload = client.pushIso(topIso)

				So(ConsolidateIsolateSources(ctx, nil, job), ShouldBeNil)

				So(sw.Task.TaskSlices[0].Properties.CasInputs, ShouldResemble, client.pushIso(client.mkIso(
					"tippy", "top",
					"left", "shark",
					"right", "shark",
					"diamonds", "forever",
				)))
			})

			Convey(`diamond includes are fine`, func() {
				sw.Task.TaskSlices[0].Properties.CasInputs = client.pushIso(client.mkIso(
					"something", "data",
				))
				job.UserPayload = client.pushIso(client.mkIso(
					"something", "conflict",
				))
				So(ConsolidateIsolateSources(ctx, nil, job), ShouldErrLike,
					`overlapping path "something"`)
			})

			Convey(`bad server in slice`, func() {
				sw.Task.TaskSlices[0].Properties.CasInputs = client.pushIso(client.mkIso(
					"something", "data",
				))
				sw.Task.TaskSlices[0].Properties.CasInputs.Server = "narp"
				job.UserPayload = &swarmingpb.CASTree{
					Server:    client.Server(),
					Namespace: client.Namespace(),
				}
				So(ConsolidateIsolateSources(ctx, nil, job), ShouldErrLike,
					"two different servers")
			})

			Convey(`bad namespace in slice`, func() {
				sw.Task.TaskSlices[0].Properties.CasInputs = client.pushIso(client.mkIso(
					"something", "data",
				))
				sw.Task.TaskSlices[0].Properties.CasInputs.Namespace = "narp"
				job.UserPayload = &swarmingpb.CASTree{
					Server:    client.Server(),
					Namespace: client.Namespace(),
				}
				So(ConsolidateIsolateSources(ctx, nil, job), ShouldErrLike,
					"two different namespaces")
			})
		})

	})
}
