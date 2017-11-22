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
//
// The given labels will be applied to each log entry. This allows to reuse
// Logger instance between requests, even if they have different default labels.
//
// An alternative is to construct Logger per request, setting request labels via
// CommonLabels(...) option. But it is more heavy solution, and at the time of
// writing it leads to memory leaks:
// https://github.com/GoogleCloudPlatform/google-cloud-go/issues/720#issuecomment-346199870
func WithLogger(c context.Context, l *cloudLogging.Logger, labels map[string]string) context.Context {
	return logging.SetFactory(c, func(c context.Context) logging.Logger {
		return &boundLogger{
			Context: c,
			cl:      l,
			labels:  labels,
		}
	})
}

type boundLogger struct {
	context.Context
	cl     *cloudLogging.Logger
	labels map[string]string
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

	// Per docs, 'Log' takes ownership of Labels map, so make a copy.
	var labels map[string]string
	if len(bl.labels) != 0 {
		labels = make(map[string]string, len(bl.labels))
		for k, v := range bl.labels {
			labels[k] = v
		}
	}

	// Generate a LogEntry for the supplied parameters.
	bl.cl.Log(cloudLogging.Entry{
		Severity: severityForLevel(lvl),
		Payload:  line,
		Labels:   labels,
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
