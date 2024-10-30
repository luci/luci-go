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

package host

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/client/cipd/plugin"
	"go.chromium.org/luci/cipd/client/cipd/plugin/plugins/admission"
	"go.chromium.org/luci/cipd/client/cipd/plugin/protocol"
	"go.chromium.org/luci/cipd/common"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

const (
	exampleHost       = "https://example.com"
	fakeMetadataLimit = 15
	listingPageSize   = 7
)

func init() {
	registerPluginMain("PLUGIN_ADMISSION", func(ctx context.Context, mode string) error {
		switch mode {
		case "NOT_CONNECTING":
			// Block until stdin closes (which indicates the host is closing us).
			io.Copy(io.Discard, os.Stdin)
			return nil
		case "CRASHING_WHEN_CONNECTING":
			os.Exit(2)
		}

		var count int32

		return admission.Run(ctx, os.Stdin, "some version", func(ctx context.Context, req *protocol.Admission, info admission.InstanceInfo) error {
			cur := atomic.AddInt32(&count, 1)

			if req.ServiceUrl != exampleHost {
				return status.Errorf(codes.FailedPrecondition, "unexpected host")
			}

			switch mode {
			case "NORMAL_REPLY":
				if strings.HasPrefix(req.Package, "good/") {
					return nil
				}
				logging.Infof(ctx, "Rejecting %s:%s:%s", req.ServiceUrl, req.Package, common.ObjectRefToInstanceID(req.Instance))
				return status.Errorf(codes.FailedPrecondition, "the plugin says boo")

			case "BLOCK_REQUEST":
				<-ctx.Done()
				return nil

			case "CRASH_ON_SECOND_REQUEST":
				if cur == 2 {
					os.Exit(2)
				}
				return nil

			case "BLOCK_ALL_BUT_FIRST":
				if cur != 1 {
					<-ctx.Done()
				}
				return nil

			case "VISIT_METADATA_ALL":
				var visited []string
				err := info.VisitMetadata(ctx, []string{"some-key"}, listingPageSize,
					func(md *api.InstanceMetadata) bool {
						visited = append(visited, string(md.Value))
						return true
					},
				)
				if err != nil {
					return err
				}
				if len(visited) != fakeMetadataLimit {
					return status.Errorf(codes.FailedPrecondition, "unexpected number of metadata entries")
				}
				for i, v := range visited {
					if v != fmt.Sprintf("metadata-value-%d", i) {
						return status.Errorf(codes.FailedPrecondition, "unexpected metadata #%d %q", i, v)
					}
				}
				return nil

			case "VISIT_METADATA_ONE":
				var visited []string
				err := info.VisitMetadata(ctx, []string{"some-key"}, 0,
					func(md *api.InstanceMetadata) bool {
						visited = append(visited, string(md.Value))
						return false
					},
				)
				if err != nil {
					return err
				}
				if len(visited) != 1 || visited[0] != "metadata-value-0" {
					return status.Errorf(codes.FailedPrecondition, "unexpected metadata %q", visited)
				}
				return nil

			default:
				return status.Errorf(codes.Aborted, "unknown mode")
			}
		})
	})
}

func TestAdmissionPlugins(t *testing.T) {
	t.Parallel()

	const testInstanceID = "qUiQTy8PR5uPgZdpSzAYSw0u0cHNKh7A-4XSmaGSpEcC"

	testObjectRef := common.InstanceIDToObjectRef(testInstanceID)

	testPin := func(pkg string) common.Pin {
		return common.Pin{
			PackageName: pkg,
			InstanceID:  testInstanceID,
		}
	}

	ctx := gologger.StdConfig.Use(context.Background())
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ftt.Run("With a host", t, func(t *ftt.Test) {
		fakeRepo := &fakeRepository{}
		host := &Host{}
		host.Initialize(plugin.Config{
			ServiceURL: exampleHost,
			Repository: fakeRepo,
		})
		defer host.Close(ctx)

		newPlugin := func(testCase string) *AdmissionPlugin {
			return NewAdmissionPlugin(ctx, host, []string{os.Args[0], "PLUGIN_ADMISSION", testCase})
		}

		t.Run("Happy path", func(t *ftt.Test) {
			plug := newPlugin("NORMAL_REPLY")
			defer plug.Close(ctx)

			good := plug.CheckAdmission(testPin("good/a/b"))
			bad := plug.CheckAdmission(testPin("bad/a/b"))

			// Reuses pending requests.
			dup := plug.CheckAdmission(testPin("good/a/b"))
			assert.Loosely(t, dup, should.Equal(good))

			// Wait until completion.
			assert.Loosely(t, good.Wait(ctx), should.BeNil)

			err := bad.Wait(ctx)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
			assert.Loosely(t, err, should.ErrLike("the plugin says boo"))

			// Caches the result.
			assert.Loosely(t, bad.Wait(ctx), should.Equal(err))

			// Reuses finished requests.
			dup = plug.CheckAdmission(testPin("good/a/b"))
			assert.Loosely(t, dup, should.Equal(good))

			// "Forget" resolved promises.
			plug.ClearCache()

			// Makes a new one now.
			anotherGood := plug.CheckAdmission(testPin("good/a/b"))
			// Note: ShouldNotEqual triggers false race condition warning because
			// GoConvey tries to read more than it should. We just want to compare
			// pointers.
			assert.Loosely(t, anotherGood != good, should.BeTrue)
			assert.Loosely(t, anotherGood.Wait(ctx), should.BeNil)

			// A bit of a stress testing.
			promises := make([]plugin.Promise, 1000)
			for i := range promises {
				promises[i] = plug.CheckAdmission(testPin(fmt.Sprintf("good/pkg/%d", i)))
			}
			for _, p := range promises {
				if err := p.Wait(ctx); err != nil {
					assert.Loosely(t, err, should.BeNil) // spam convey only on failures
				}
			}

			plug.Close(ctx)

			// Rejects all requests right away if closed.
			p := plug.CheckAdmission(testPin("good/a/b/c/d/e"))
			assert.Loosely(t, p.Wait(ctx), should.Equal(ErrAborted))
		})

		t.Run("VisitMetadata visit all", func(t *ftt.Test) {
			plug := newPlugin("VISIT_METADATA_ALL")
			defer plug.Close(ctx)
			assert.Loosely(t, plug.CheckAdmission(testPin("good/a/b")).Wait(ctx), should.BeNil)
			assert.Loosely(t, fakeRepo.Calls(), should.Resemble([]*api.ListMetadataRequest{
				{
					Package:  "good/a/b",
					Instance: testObjectRef,
					Keys:     []string{"some-key"},
					PageSize: listingPageSize,
				},
				{
					Package:   "good/a/b",
					Instance:  testObjectRef,
					Keys:      []string{"some-key"},
					PageSize:  listingPageSize,
					PageToken: "start-from-7",
				},
				{
					Package:   "good/a/b",
					Instance:  testObjectRef,
					Keys:      []string{"some-key"},
					PageSize:  listingPageSize,
					PageToken: "start-from-14",
				},
			}))
		})

		t.Run("VisitMetadata visit one", func(t *ftt.Test) {
			plug := newPlugin("VISIT_METADATA_ONE")
			defer plug.Close(ctx)
			assert.Loosely(t, plug.CheckAdmission(testPin("good/a/b")).Wait(ctx), should.BeNil)
			assert.Loosely(t, fakeRepo.Calls(), should.Resemble([]*api.ListMetadataRequest{
				{
					Package:  "good/a/b",
					Instance: testObjectRef,
					Keys:     []string{"some-key"},
					PageSize: 20, // default
				},
			}))
		})

		t.Run("VisitMetadata visit error", func(t *ftt.Test) {
			fakeRepo.SetErr(status.Errorf(codes.PermissionDenied, "the listing says boo"))

			plug := newPlugin("VISIT_METADATA_ALL")
			defer plug.Close(ctx)

			err := plug.CheckAdmission(testPin("good/a/b")).Wait(ctx)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("the listing says boo"))

			assert.Loosely(t, fakeRepo.Calls(), should.Resemble([]*api.ListMetadataRequest{
				{
					Package:  "good/a/b",
					Instance: testObjectRef,
					Keys:     []string{"some-key"},
					PageSize: listingPageSize,
				},
			}))
		})

		t.Run("Closing right after starting", func(t *ftt.Test) {
			plug := newPlugin("BLOCK_REQUEST")
			p := plug.CheckAdmission(testPin("good/a/b"))
			plug.Close(ctx)

			// The exact error depends on how far we progressed before the plugin
			// was closed. Either way it should NOT be a context deadline.
			err := p.Wait(ctx)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, ctx.Err(), should.BeNil)
		})

		t.Run("Plugin is not connecting", func(t *ftt.Test) {
			plug := newPlugin("NOT_CONNECTING")
			plug.timeout = time.Second
			defer plug.Close(ctx)

			t.Run("Timeout", func(t *ftt.Test) {
				p := plug.CheckAdmission(testPin("good/a/b"))
				assert.Loosely(t, p.Wait(ctx), should.NotBeNil)
				assert.Loosely(t, ctx.Err(), should.BeNil)
			})

			t.Run("Closing while waiting", func(t *ftt.Test) {
				p := plug.CheckAdmission(testPin("good/a/b"))
				plug.Close(ctx)
				assert.Loosely(t, p.Wait(ctx), should.NotBeNil)
				assert.Loosely(t, ctx.Err(), should.BeNil)
			})
		})

		t.Run("Plugin is not found", func(t *ftt.Test) {
			plug := NewAdmissionPlugin(ctx, host, []string{"doesnt_exist"})
			defer plug.Close(ctx)

			p := plug.CheckAdmission(testPin("good/a/b"))
			assert.Loosely(t, p.Wait(ctx), should.NotBeNil)
			assert.Loosely(t, ctx.Err(), should.BeNil)
		})

		t.Run("Plugin is using unexpected protocol version", func(t *ftt.Test) {
			plug := newPlugin("NORMAL_REPLY")
			plug.protocolVersion = 666
			defer plug.Close(ctx)

			p := plug.CheckAdmission(testPin("good/a/b"))
			assert.Loosely(t, p.Wait(ctx), should.NotBeNil)
			assert.Loosely(t, ctx.Err(), should.BeNil)
		})

		t.Run("Plugin is crashing when connecting", func(t *ftt.Test) {
			plug := newPlugin("CRASHING_WHEN_CONNECTING")
			defer plug.Close(ctx)

			p := plug.CheckAdmission(testPin("good/a/b"))
			assert.Loosely(t, p.Wait(ctx), should.NotBeNil)
			assert.Loosely(t, ctx.Err(), should.BeNil)
		})

		t.Run("Plugin is crashing midway", func(t *ftt.Test) {
			plug := newPlugin("CRASH_ON_SECOND_REQUEST")
			defer plug.Close(ctx)

			p1 := plug.CheckAdmission(testPin("good/a/b/1"))
			assert.Loosely(t, p1.Wait(ctx), should.BeNil)

			p2 := plug.CheckAdmission(testPin("good/a/b/2"))
			assert.Loosely(t, p2.Wait(ctx), should.NotBeNil)
		})

		t.Run("Terminating with pending queue", func(t *ftt.Test) {
			plug := newPlugin("BLOCK_ALL_BUT_FIRST")
			defer plug.Close(ctx)

			p1 := plug.CheckAdmission(testPin("good/a/b/1"))
			assert.Loosely(t, p1.Wait(ctx), should.BeNil)

			p2 := plug.CheckAdmission(testPin("good/a/b/2"))
			p3 := plug.CheckAdmission(testPin("good/a/b/3"))

			plug.Close(ctx)

			assert.Loosely(t, p2.Wait(ctx), should.NotBeNil)
			assert.Loosely(t, p3.Wait(ctx), should.NotBeNil)
			assert.Loosely(t, ctx.Err(), should.BeNil)
		})
	})
}

type fakeRepository struct {
	m     sync.Mutex
	err   error
	calls []*api.ListMetadataRequest
}

func (r *fakeRepository) SetErr(err error) {
	r.m.Lock()
	defer r.m.Unlock()
	r.err = err
}

func (r *fakeRepository) Calls() []*api.ListMetadataRequest {
	r.m.Lock()
	defer r.m.Unlock()
	return r.calls
}

func (r *fakeRepository) ListMetadata(ctx context.Context, in *api.ListMetadataRequest, opts ...grpc.CallOption) (*api.ListMetadataResponse, error) {
	r.m.Lock()
	r.calls = append(r.calls, in)
	err := r.err
	r.m.Unlock()

	if err != nil {
		return nil, err
	}

	cursor := 0
	if in.PageToken != "" {
		fmt.Sscanf(in.PageToken, "start-from-%d", &cursor)
	}

	key := "some-key"
	if len(in.Keys) != 0 {
		key = in.Keys[0]
	}

	var md []*api.InstanceMetadata
	for cursor < fakeMetadataLimit && len(md) < int(in.PageSize) {
		md = append(md, &api.InstanceMetadata{
			Key:         key,
			Value:       []byte(fmt.Sprintf("metadata-value-%d", cursor)),
			ContentType: "text/plain",
		})
		cursor += 1
	}

	nextPageToken := ""
	if cursor != fakeMetadataLimit {
		nextPageToken = fmt.Sprintf("start-from-%d", cursor)
	}

	return &api.ListMetadataResponse{
		Metadata:      md,
		NextPageToken: nextPageToken,
	}, nil
}
