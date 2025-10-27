// Copyright 2025 The LUCI Authors.
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

package bqexport

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"

	"go.chromium.org/luci/auth_service/api/configspb"
	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/settingscfg"
)

func TestBQExportSettings(t *testing.T) {
	t.Parallel()

	ftt.Run("Run respects settings.cfg", t, func(t *ftt.Test) {
		t.Run("disabled if not set in settings.cfg", func(t *ftt.Test) {
			ctx := memory.Use(context.Background())
			ctx = memlogger.Use(logging.SetLevel(ctx, logging.Info))
			logger := logging.Get(ctx)

			// Set up empty settings config.
			cfg := &configspb.SettingsCfg{}
			assert.Loosely(t, settingscfg.SetInTest(ctx, cfg, nil), should.BeNil)
			assert.Loosely(t, Run(ctx), should.BeNil)
			ml := logger.(*memlogger.MemLogger)
			assert.Loosely(t, ml,
				convey.Adapt(memlogger.ShouldHaveLog)(logging.Info, "BQ export is disabled"))
		})
	})
}

func TestExportSupplementalData(t *testing.T) {
	t.Parallel()

	ftt.Run("exportSupplementalData gets latest configs", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		testTime := timestamppb.New(
			time.Date(2020, time.August, 16, 15, 20, 0, 0, time.UTC))

		t.Run("fails without permissions config", func(t *ftt.Test) {
			err := exportSupplementalData(ctx, nil, testTime)
			assert.Loosely(t, err, should.NotBeNil)
		})
	})
}
