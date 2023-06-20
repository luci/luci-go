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
	"compress/gzip"
	"context"
	"strconv"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/config_service/internal/model"

	. "github.com/smartystreets/goconvey/convey"
)

const AppID = "luci-config-dev"

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
	// be explicitly added
	datastore.GetTestable(ctx).AutoIndex(false)
	return ctx
}

// InjectConfigSet writes a new revision for the provided config set.
//
// The revision ID is a monotonically increasing integer.
func InjectConfigSet(ctx context.Context, cfgSet config.Set, configs map[string]proto.Message) {
	cs := &model.ConfigSet{
		ID: cfgSet,
	}
	switch err := datastore.Get(ctx, cs); err {
	case datastore.ErrNoSuchEntity:
		cs.LatestRevision.ID = "1"
	case nil:
		prevID, err := strconv.Atoi(cs.LatestRevision.ID)
		So(err, ShouldBeNil)
		cs.LatestRevision.ID = strconv.Itoa(prevID + 1)
	default:
		So(err, ShouldBeNil)
	}

	var files []*model.File
	for filepath, msg := range configs {
		content, err := prototext.Marshal(msg)
		So(err, ShouldBeNil)
		var compressed bytes.Buffer
		gw := gzip.NewWriter(&compressed)
		_, err = gw.Write(content)
		So(err, ShouldBeNil)
		So(gw.Close(), ShouldBeNil)
		files = append(files, &model.File{
			Path:     filepath,
			Revision: datastore.MakeKey(ctx, model.ConfigSetKind, string(cfgSet), model.RevisionKind, cs.LatestRevision.ID),
			Content:  compressed.Bytes(),
		})
	}
	So(datastore.Put(ctx, cs, files), ShouldBeNil)
}

// InjectSelfConfigs is a shorthand for updating LUCI Config self config.
func InjectSelfConfigs(ctx context.Context, configs map[string]proto.Message) {
	InjectConfigSet(ctx, config.MustServiceSet(AppID), configs)
}
