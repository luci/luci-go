// Copyright 2022 The LUCI Authors.
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

package bbfake

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	bbutil "go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/buildbucket"
	"go.chromium.org/luci/cv/internal/common"
)

const requestDeduplicationWindow = 1 * time.Minute

type fakeApp struct {
	hostname           string
	nextBuildID        int64 // for generating monotonically decreasing build ID
	requestCache       timedMap
	pubsubPushEndpoint string
	buildStoreMu       sync.RWMutex
	buildStore         map[int64]*bbpb.Build // build ID -> build
	configStoreMu      sync.RWMutex
	configStore        map[string]*bbpb.BuildbucketCfg // project name -> config
}

type Fake struct {
	hostsMu sync.RWMutex
	hosts   map[string]*fakeApp // hostname -> fakeApp
}

// NewClientFactory returns a factory that creates a client for this buildbucket
// fake.
func (f *Fake) NewClientFactory() buildbucket.ClientFactory {
	return clientFactory{
		fake: f,
	}
}

// MustNewClient is a shorthand of `fake.NewClientFactory().MakeClient(...)`.
//
// Panics if fails to create new client.
func (f *Fake) MustNewClient(ctx context.Context, host, luciProject string) *Client {
	factory := clientFactory{
		fake: f,
	}
	client, err := factory.MakeClient(ctx, host, luciProject)
	if err != nil {
		panic(errors.Fmt("failed to create new buildbucket client: %w", err))
	}
	return client.(*Client)
}

// RegisterPubSubPushEndpoint registers a Pub/Sub push endpoint for the given host.
//
// If a build is updated to the terminal status, an event will be synthesized
// and send to the push endpoint for processing
func (f *Fake) RegisterPubSubPushEndpoint(host string, pushEndpoint string) {
	fa := f.ensureApp(host)
	fa.pubsubPushEndpoint = pushEndpoint
}

// AddBuilder adds a new builder configuration to fake Buildbucket host.
//
// Overwrites the existing builder if the same builder already exists.
// `properties` should be marshallable by `encoding/json`.
func (f *Fake) AddBuilder(host string, builder *bbpb.BuilderID, properties any) *Fake {
	fa := f.ensureApp(host)
	fa.configStoreMu.Lock()
	defer fa.configStoreMu.Unlock()
	if _, ok := fa.configStore[builder.GetProject()]; !ok {
		fa.configStore[builder.GetProject()] = &bbpb.BuildbucketCfg{}
	}
	cfg := fa.configStore[builder.GetProject()]
	var bucket *bbpb.Bucket
	for _, b := range cfg.GetBuckets() {
		if b.Name == builder.GetBucket() {
			bucket = b
			break
		}
	}
	if bucket == nil {
		bucket = &bbpb.Bucket{
			Name:     builder.GetBucket(),
			Swarming: &bbpb.Swarming{},
		}
		cfg.Buckets = append(cfg.GetBuckets(), bucket)
	}

	builderCfg := &bbpb.BuilderConfig{
		Name: builder.GetBuilder(),
	}
	if properties != nil {
		bProperties, err := json.Marshal(properties)
		if err != nil {
			panic(err)
		}
		builderCfg.Properties = string(bProperties)
	}
	for i, b := range bucket.GetSwarming().GetBuilders() {
		if b.Name == builder.GetBuilder() {
			bucket.GetSwarming().GetBuilders()[i] = builderCfg
			return f
		}
	}
	bucket.GetSwarming().Builders = append(bucket.GetSwarming().GetBuilders(), builderCfg)
	return f
}

// EnsureBuilders ensures all builders defined in the Project config are added
// to the Buildbucket fake.
func (f *Fake) EnsureBuilders(cfg *cfgpb.Config) {
	added := stringset.New(1)
	for _, cg := range cfg.GetConfigGroups() {
		for _, b := range cg.GetVerifiers().GetTryjob().GetBuilders() {
			if added.Has(fmt.Sprintf("%s/%s", b.GetHost(), b.GetName())) {
				continue
			}
			builder, err := bbutil.ParseBuilderID(b.GetName())
			if err != nil {
				panic(err)
			}
			f.AddBuilder(b.GetHost(), builder, nil)
			added.Add(fmt.Sprintf("%s/%s", b.GetHost(), b.GetName()))
		}
	}
}

// MutateBuild mutates the provided build.
//
// Panics if the provided build is not found.
func (f *Fake) MutateBuild(ctx context.Context, host string, id int64, mutateFn func(*bbpb.Build)) *bbpb.Build {
	f.hostsMu.RLock()
	fakeApp, ok := f.hosts[host]
	f.hostsMu.RUnlock()
	if !ok {
		panic(errors.Fmt("unknown host %q", host))
	}
	return fakeApp.updateBuild(ctx, id, mutateFn)
}

func (f *Fake) ensureApp(host string) *fakeApp {
	f.hostsMu.Lock()
	defer f.hostsMu.Unlock()
	if _, ok := f.hosts[host]; !ok {
		if f.hosts == nil {
			f.hosts = make(map[string]*fakeApp)
		}
		f.hosts[host] = &fakeApp{
			hostname:    host,
			nextBuildID: math.MaxInt64 - 1,
			buildStore:  make(map[int64]*bbpb.Build),
			configStore: make(map[string]*bbpb.BuildbucketCfg),
		}
	}
	return f.hosts[host]
}

func (fa *fakeApp) getBuild(id int64) *bbpb.Build {
	fa.buildStoreMu.RLock()
	defer fa.buildStoreMu.RUnlock()
	if build, ok := fa.buildStore[id]; ok {
		return proto.Clone(build).(*bbpb.Build)
	}
	return nil
}

func (fa *fakeApp) iterBuildStore(cb func(*bbpb.Build)) {
	fa.buildStoreMu.RLock()
	defer fa.buildStoreMu.RUnlock()
	for _, build := range fa.buildStore {
		cb(proto.Clone(build).(*bbpb.Build))
	}
}

func (fa *fakeApp) updateBuild(ctx context.Context, id int64, cb func(*bbpb.Build)) *bbpb.Build {
	fa.buildStoreMu.Lock()
	defer fa.buildStoreMu.Unlock()
	if build, ok := fa.buildStore[id]; ok {
		cb(build)
		build.UpdateTime = timestamppb.New(clock.Now(ctx).UTC())
		// store a copy to avoid cb keeps the reference to the build and mutate it
		// later.
		fa.buildStore[id] = proto.Clone(build).(*bbpb.Build)
		fa.pushToSubscriberIfNecessary(build)
		return build
	}
	panic(errors.Fmt("unknown build %d", id))
}

// insertBuild also generates a monotonically decreasing build ID.
//
// Caches the build for `requestDeduplicationWindow` to deduplicate request
// with same request ID later.
func (fa *fakeApp) insertBuild(ctx context.Context, build *bbpb.Build, requestID string) *bbpb.Build {
	fa.buildStoreMu.Lock()
	defer fa.buildStoreMu.Unlock()
	build.Id = fa.nextBuildID
	fa.nextBuildID--
	if _, ok := fa.buildStore[build.Id]; ok {
		panic(fmt.Sprintf("build %d already exists", build.Id))
	}
	cloned := proto.Clone(build).(*bbpb.Build)
	fa.buildStore[build.Id] = cloned
	if requestID != "" {
		fa.requestCache.set(ctx, requestID, cloned, requestDeduplicationWindow)
	}
	fa.pushToSubscriberIfNecessary(build)
	return build
}

func (fa *fakeApp) pushToSubscriberIfNecessary(build *bbpb.Build) {
	if fa.pubsubPushEndpoint == "" || !bbutil.IsEnded(build.GetStatus()) {
		return
	}
	msg := &bbpb.BuildsV2PubSub{
		Build: build,
	}
	data, err := protojson.Marshal(msg)
	if err != nil {
		panic(errors.Fmt("failed to marshal pubsub message: %w", err))
	}

	payload := common.PubSubMessagePayload{
		Message: common.PubSubMessage{
			Data: base64.StdEncoding.EncodeToString(data),
		},
	}
	payloadJson, err := json.Marshal(payload)
	if err != nil {
		panic(errors.Fmt("failed to marshal pubsub message to json: %w", err))
	}
	switch resp, err := http.Post(fa.pubsubPushEndpoint, "application/json", bytes.NewBuffer(payloadJson)); {
	case err != nil:
		panic(errors.Fmt("failed to push pubsub message: %w", err))
	case resp.StatusCode != http.StatusOK:
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			panic(errors.Fmt("failed to parse response body: %w", err))
		}
		panic(errors.Fmt("failed to push pubsub message, status code: %d, body: %s", resp.StatusCode, body))
	}
}

func (fa *fakeApp) findDupRequest(ctx context.Context, requestID string) *bbpb.Build {
	if requestID == "" {
		return nil
	}
	if b, ok := fa.requestCache.get(ctx, requestID); ok {
		return proto.Clone(b.(*bbpb.Build)).(*bbpb.Build)
	}
	return nil
}

func (fa *fakeApp) loadBuilderCfg(builderID *bbpb.BuilderID) *bbpb.BuilderConfig {
	fa.configStoreMu.RLock()
	defer fa.configStoreMu.RUnlock()
	cfg, ok := fa.configStore[builderID.GetProject()]
	if !ok {
		return nil
	}
	for _, bucket := range cfg.GetBuckets() {
		if bucket.GetName() != builderID.GetBucket() {
			continue
		}
		for _, builder := range bucket.GetSwarming().GetBuilders() {
			if builder.GetName() == builderID.GetBuilder() {
				return proto.Clone(builder).(*bbpb.BuilderConfig)
			}
		}
	}
	return nil
}
