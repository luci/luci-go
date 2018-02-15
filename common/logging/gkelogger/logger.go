// Copyright 2018 The LUCI Authors.
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

package gkelogger

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"golang.org/x/net/context"
)

func jsonLoggerFactory(c context.Context) logging.Logger {
	return jsonLogger{
		ctx: c,
		out: os.Stdout,
	}
}

type jsonLogger struct {
	ctx context.Context
	out io.Writer
}

func (l jsonLogger) Debugf(format string, args ...interface{}) {
	l.LogCall(logging.Debug, 1, format, args)
}

func (l jsonLogger) Infof(format string, args ...interface{}) {
	l.LogCall(logging.Info, 1, format, args)
}

func (l jsonLogger) Warningf(format string, args ...interface{}) {
	l.LogCall(logging.Warning, 1, format, args)
}

func (l jsonLogger) Errorf(format string, args ...interface{}) {
	l.LogCall(logging.Error, 1, format, args)
}

func (l jsonLogger) LogCall(lv logging.Level, calldepth int, format string, args []interface{}) {
	message := fmt.Sprintf(format, args...)
	fields := logging.GetFields(l.ctx)
	if fields == nil {
		fields = logging.Fields{}
	}
	// This is magic, see: https://github.com/GoogleCloudPlatform/fluent-plugin-google-cloud/blob/api_v2/lib/fluent/plugin/out_google_cloud.rb#L900
	fields["severity"] = lv.String()
	// This is magic, see: https://github.com/GoogleCloudPlatform/fluent-plugin-google-cloud/blob/api_v2/lib/fluent/plugin/out_google_cloud.rb#L1123
	fields["message"] = message
	// This is magic, see: https://github.com/GoogleCloudPlatform/fluent-plugin-google-cloud/blob/api_v2/lib/fluent/plugin/out_google_cloud.rb#L883
	fields["time"] = clock.Now(l.ctx).Format(time.RFC3339Nano)
	// Add a custom error field.
	if err, ok := fields[logging.ErrorKey]; ok {
		if ierr, ok := err.(error); ok {
			fields["error"] = ierr.Error()
		}
	}
	if err := json.NewEncoder(l.out).Encode(fields); err != nil {
		panic(err)
	}
}
