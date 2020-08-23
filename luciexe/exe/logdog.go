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

package exe

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"sync"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	ldtypes "go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/luciexe"
)

// addUniqueLog is a helper to generate and append Log entries to both
// Build.Output.Logs and Step.Logs.
//
// It is guaranteed to produce unique logdog stream names with the following
// rules:
//   * Build logs have the stream name "l/$logid"
//   * Step logs have the stream name  "s/$stepIndex/l/$logid"
//   * Step logdog namespaces are      "s/$stepIndex/u"
//
// `stepIdx` is the step index; If it's < 0, then this generates a "Build" log
//
// Where $logid is:
//   * The given `name` of the log, IFF `name` can be expressed without
//     alteration in logdog's streamname alphabet AND `name` is not numeric.
//   * Otherwise, its the index of the log in its context (either
//     Build.Output.Log or Step.Log).
//
// Used by Build.Log* and Step.Log*
func addUniqueLog(stepIdx int, name string, logManipulator func(func(logs *[]*bbpb.Log) error) error) (fullName ldtypes.StreamName, err error) {
	err = logManipulator(func(logs *[]*bbpb.Log) error {
		for _, log := range *logs {
			if log.Name == name {
				return errors.Reason("duplicate logname %q", name).Tag(DuplicateLogTag).Err()
			}
		}

		logID := name
		_, isNumeric := strconv.ParseUint(logID, 10, 64)
		isValidStreamName := ldtypes.StreamName(logID).Validate()
		if isNumeric == nil || isValidStreamName != nil {
			logID = fmt.Sprintf("%d", len(*logs))
		}

		if stepIdx < 0 {
			fullName = ldtypes.StreamName(fmt.Sprintf("l/%s", logID))
		} else {
			fullName = ldtypes.StreamName(fmt.Sprintf("s/%d/l/%s", stepIdx, logID))
		}
		if err = fullName.Validate(); err != nil {
			// this is "impossible" due to the way that we've constructed fullName.
			panic(errors.Annotate(err, "logdog streamname %q", fullName).Err())
		}

		*logs = append(*logs, &bbpb.Log{
			Name: name,
			Url:  string(fullName), // see luciexe docs re: Url
		})
		return nil
	})
	return fullName, err
}

func synthesizeLogdogNamespace(ctx context.Context, processNamespace ldtypes.StreamName, stepIdx int) context.Context {
	env := environ.FromCtx(ctx)
	if processNamespace != "" {
		env[luciexe.LogdogNamespaceEnv] = fmt.Sprintf("%s/s/%d/u", processNamespace, stepIdx)
	} else {
		env[luciexe.LogdogNamespaceEnv] = fmt.Sprintf("s/%d/u", stepIdx)
	}
	return environ.With(ctx, env)
}

type loggable interface {
	Log(ctx context.Context, name string, opts ...streamclient.Option) (io.WriteCloser, error)
}

func lazyLogger(l loggable, stepName string) (logging.Factory, func() error) {
	var init sync.Once
	var fd io.WriteCloser
	var logCfg gologger.LoggerConfig

	return func(ctx context.Context) logging.Logger {
			init.Do(func() {
				var err error
				fd, err = l.Log(ctx, "log")
				if DuplicateLogTag.In(err) {
					return
				} else if err != nil {
					nameCtx := "build"
					if stepName != "" {
						nameCtx = fmt.Sprintf("step %q", stepName)
					}
					panic(errors.Annotate(
						err, "creating `log` stream for %s", nameCtx).Err())
				}
				logCfg = gologger.LoggerConfig{Out: fd, Format: gologger.StdFormat}
			})
			if logCfg.Out == nil {
				return logging.Null
			}
			return logCfg.NewLogger(ctx)
		}, func() error {
			if fd != nil {
				return fd.Close()
			}
			return nil
		}
}
