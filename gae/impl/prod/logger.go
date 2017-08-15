// Copyright 2015 The LUCI Authors.
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

package prod

import (
	"fmt"

	"go.chromium.org/luci/common/logging"

	"google.golang.org/appengine/log"

	"golang.org/x/net/context"
)

// useLogging adds a logging.Logger implementation to the context which logs to
// appengine's log handler.
func useLogging(c context.Context) context.Context {
	return logging.SetFactory(c, func(ic context.Context) logging.Logger {
		return &loggerImpl{getAEContext(ic), ic}
	})
}

type loggerImpl struct {
	aeCtx context.Context
	ic    context.Context
}

func (gl *loggerImpl) Debugf(format string, args ...interface{}) {
	gl.LogCall(logging.Debug, 1, format, args)
}
func (gl *loggerImpl) Infof(format string, args ...interface{}) {
	gl.LogCall(logging.Info, 1, format, args)
}
func (gl *loggerImpl) Warningf(format string, args ...interface{}) {
	gl.LogCall(logging.Warning, 1, format, args)
}
func (gl *loggerImpl) Errorf(format string, args ...interface{}) {
	gl.LogCall(logging.Error, 1, format, args)
}

// TODO(riannucci): prefix with caller's code location.
func (gl *loggerImpl) LogCall(l logging.Level, calldepth int, format string, args []interface{}) {
	if gl.aeCtx == nil || !logging.IsLogging(gl.ic, l) {
		return
	}

	var logf func(context.Context, string, ...interface{})
	switch l {
	case logging.Debug:
		logf = log.Debugf
	case logging.Info:
		logf = log.Infof
	case logging.Warning:
		logf = log.Warningf

	case logging.Error:
		fallthrough
	default:
		logf = log.Errorf
	}

	fields := logging.GetFields(gl.ic)
	if len(fields) > 0 {
		logf(gl.aeCtx, "%s :: %s", fmt.Sprintf(format, args...), fields.String())
	} else {
		logf(gl.aeCtx, format, args...)
	}
}
