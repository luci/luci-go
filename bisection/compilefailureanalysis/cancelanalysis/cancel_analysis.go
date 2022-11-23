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

// Package cancelanalysis handles cancelation of existing analyses.
package cancelanalysis

import (
	"context"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/tq"

	tpb "go.chromium.org/luci/bisection/task/proto"
)

const (
	taskClass = "cancel-analysis"
	queue     = "cancel-analysis"
)

// RegisterTaskClass registers the task class for tq dispatcher.
func RegisterTaskClass() {
	tq.RegisterTaskClass(tq.TaskClass{
		ID:        taskClass,
		Prototype: (*tpb.CancelAnalysisTask)(nil),
		Queue:     queue,
		Kind:      tq.NonTransactional,
		Handler: func(c context.Context, payload proto.Message) error {
			task := payload.(*tpb.CancelAnalysisTask)
			logging.Infof(c, "Process CancelAnalysisTask with id = %d", task.GetAnalysisId())
			err := CancelAnalysis(c, task.GetAnalysisId())
			if err != nil {
				err := errors.Annotate(err, "cancelAnalysis id=%d", task.GetAnalysisId()).Err()
				logging.Errorf(c, err.Error())
				// If the error is transient, return err to retry
				if transient.Tag.In(err) {
					return err
				}
				return nil
			}
			return nil
		},
	})
}

// CancelAnalysis cancels all pending and running reruns for an analysis.
func CancelAnalysis(c context.Context, analysisID int64) error {
	// TODO (nqmtuan): Implement this
	return nil
}
