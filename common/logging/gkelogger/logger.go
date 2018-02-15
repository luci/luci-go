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
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"golang.org/x/net/context"
)

type LoggerConfig struct {
	Out io.Writer
}

func (lc *LoggerConfig) getJsonLoggerFactory() func(context.Context) logging.Logger {
	return func(c context.Context) logging.Logger {
		return jsonLogger{
			ctx: c,
			out: lc.Out,
		}
	}
}

type jsonLogger struct {
	ctx context.Context
	out io.Writer
}

// LogEntry is created from a single log entry, and is formatted in such
// a way that the GKE stack expects.
type LogEntry struct {
	// Severity is a string denoting the logging level of the entry.
	// This is magic, see: https://github.com/GoogleCloudPlatform/fluent-plugin-google-cloud/blob/api_v2/lib/fluent/plugin/out_google_cloud.rb#L900
	Severity string `json:"severity"`
	// Message is a single line human readable string of the log message.
	// This is magic, see: https://github.com/GoogleCloudPlatform/fluent-plugin-google-cloud/blob/api_v2/lib/fluent/plugin/out_google_cloud.rb#L1123
	Message string `json:"message"`
	// Time is a RFC3389Nano formatted string of the timestamp of the log.
	// This is magic, see: https://github.com/GoogleCloudPlatform/fluent-plugin-google-cloud/blob/api_v2/lib/fluent/plugin/out_google_cloud.rb#L883
	Time string `json:"time"`
	// Fields are extra structured data that may be tagged in the log entry.
	Fields logging.Fields `json:"fields"`
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
	entry := LogEntry{
		Severity: lv.String(),
		Message:  fmt.Sprintf(format, args...),
		Time:     clock.Now(l.ctx).Format(time.RFC3339Nano),
		Fields:   logging.GetFields(l.ctx),
	}
	if err := json.NewEncoder(l.out).Encode(entry); err != nil {
		panic(err)
	}
}
