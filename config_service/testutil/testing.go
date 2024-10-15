// Copyright 2023 The LUCI Authors.
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

// Package testutil reduces the boilerplate for testing in LUCI Config.
package testutil

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/klauspost/compress/gzip"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/gcloud/gs"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/auth/signing/signingtest"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/config_service/internal/model"
)

const AppID = "luci-config-dev"
const ServiceAccount = "luci-config@luci-config-dev.iam.gserviceaccount.com"
const TestGsBucket = "test-bucket"

// SetupContext sets up testing common context for LUCI Config tests.
func SetupContext() context.Context {
	ctx := context.Background()
	utc := time.Date(2023, time.March, 4, 17, 30, 00, 0, time.UTC)
	ctx, _ = testclock.UseTime(ctx, utc)
	if testing.Verbose() {
		ctx = logging.SetLevel(gologger.StdConfig.Use(ctx), logging.Debug)
	} else {
		ctx = logging.SetLevel(gologger.StdConfig.Use(ctx), logging.Info)
	}

	ctx = memory.UseWithAppID(ctx, "dev~"+AppID)
	datastore.GetTestable(ctx).Consistent(true)
	// Intentionally not enabling AutoIndex to ensure the composite index must
	// be explicitly added to index.yaml.
	datastore.GetTestable(ctx).AutoIndex(false)
	path, err := os.Getwd()
	if err != nil {
		panic(fmt.Errorf("can not get the cwd: %w", err))
	}
	for filepath.Base(path) != "config_service" {
		path = filepath.Dir(path)
	}
	if path == "." {
		panic(errors.New("can not find the root of config_service; may be the package is renamed?"))
	}

	file, err := os.Open(filepath.Join(path, "cmd", "config_server", "index.yaml"))
	if err != nil {
		panic(fmt.Errorf("failed to open index.yaml file: %w", err))
	}
	indexDefs, err := datastore.ParseIndexYAML(file)
	if err != nil {
		panic(fmt.Errorf("failed to parse index.yaml file: %w", err))
	}
	datastore.GetTestable(ctx).AddIndexes(indexDefs...)

	ctx = caching.WithEmptyProcessCache(ctx)
	ctx = auth.ModifyConfig(ctx, func(cfg auth.Config) auth.Config {
		cfg.Signer = signingtest.NewSigner(&signing.ServiceInfo{
			AppID:              AppID,
			ServiceAccountName: ServiceAccount,
		})
		return cfg
	})
	return ctx
}

// InjectConfigSet writes a new revision for the provided config set.
//
// The revision ID is a monotonically increasing integer.
func InjectConfigSet(ctx context.Context, t testing.TB, cfgSet config.Set, configs map[string]proto.Message) {
	t.Helper()
	cs := &model.ConfigSet{
		ID: cfgSet,
	}
	switch err := datastore.Get(ctx, cs); err {
	case datastore.ErrNoSuchEntity:
		cs.LatestRevision.ID = "1"
	case nil:
		prevID, err := strconv.Atoi(cs.LatestRevision.ID)
		assert.Loosely(t, err, should.BeNil, truth.LineContext())
		cs.LatestRevision.ID = strconv.Itoa(prevID + 1)
	default:
		assert.Loosely(t, err, should.BeNil, truth.LineContext())
	}
	cs.LatestRevision.CommitTime = clock.Now(ctx)

	var files []*model.File
	for filepath, msg := range configs {
		content, err := prototext.Marshal(msg)
		assert.Loosely(t, err, should.BeNil, truth.LineContext())
		var compressed bytes.Buffer
		gw := gzip.NewWriter(&compressed)
		sha := sha256.New()
		mw := io.MultiWriter(sha, gw)
		_, err = mw.Write(content)
		assert.Loosely(t, err, should.BeNil, truth.LineContext())
		assert.Loosely(t, gw.Close(), should.BeNil, truth.LineContext())
		files = append(files, &model.File{
			Path:          filepath,
			Revision:      datastore.MakeKey(ctx, model.ConfigSetKind, string(cfgSet), model.RevisionKind, cs.LatestRevision.ID),
			Content:       compressed.Bytes(),
			ContentSHA256: hex.EncodeToString(sha.Sum(nil)),
			Size:          int64(len(content)),
			GcsURI:        gs.MakePath(TestGsBucket, filepath),
		})
	}
	assert.Loosely(t, datastore.Put(ctx, cs, files), should.BeNil, truth.LineContext())
}

// InjectSelfConfigs is a shorthand for updating LUCI Config self config.
func InjectSelfConfigs(ctx context.Context, t testing.TB, configs map[string]proto.Message) {
	t.Helper()
	InjectConfigSet(ctx, t, config.MustServiceSet(AppID), configs)
}
