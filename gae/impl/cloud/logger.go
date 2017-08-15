// Copyright 2017 The LUCI Authors.
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

package cloud

import (
	"fmt"
	"regexp"

	"go.chromium.org/luci/common/logging"

	cloudLogging "cloud.google.com/go/logging"

	"golang.org/x/net/context"
)

// LogIDRegexp is a regular expression that can be used to validate a Log ID.
//
// From: https://godoc.org/cloud.google.com/go/logging#Client.Logger
var LogIDRegexp = regexp.MustCompile(`^[A-Za-z0-9/_\-\.]{0,512}$`)

// WithLogger installs an instance of a logging.Logger that forwards logs to an
// underlying StackDriver Logger into the supplied Context.
func WithLogger(c context.Context, l *cloudLogging.Logger) context.Context {
	return logging.SetFactory(c, func(c context.Context) logging.Logger {
		return &boundLogger{
			Context: c,
			cl:      l,
		}
	})
}

type boundLogger struct {
	context.Context
	cl *cloudLogging.Logger
}

func (bl *boundLogger) Debugf(format string, args ...interface{}) {
	bl.LogCall(logging.Debug, 1, format, args)
}

func (bl *boundLogger) Infof(format string, args ...interface{}) {
	bl.LogCall(logging.Info, 1, format, args)
}

func (bl *boundLogger) Warningf(format string, args ...interface{}) {
	bl.LogCall(logging.Warning, 1, format, args)
}

func (bl *boundLogger) Errorf(format string, args ...interface{}) {
	bl.LogCall(logging.Error, 1, format, args)
}

func (bl *boundLogger) LogCall(lvl logging.Level, calldepth int, format string, args []interface{}) {
	fields := logging.GetFields(bl)
	line := fmt.Sprintf(format, args...)
	if len(fields) > 0 {
		line = fmt.Sprintf("%s :: %s", line, fields.String())
	}

	// Generate a LogEntry for the supplied parameters.
	bl.cl.Log(cloudLogging.Entry{
		Severity: severityForLevel(lvl),
		Payload:  line,
	})
}

func severityForLevel(l logging.Level) cloudLogging.Severity {
	switch l {
	case logging.Debug:
		return cloudLogging.Debug
	case logging.Info:
		return cloudLogging.Info
	case logging.Warning:
		return cloudLogging.Warning
	case logging.Error:
		return cloudLogging.Error
	default:
		return cloudLogging.Default
	}
}
