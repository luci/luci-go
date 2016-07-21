// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package jobsim

import (
	"encoding/json"

	"github.com/luci/luci-go/appengine/cmd/dm/distributor"
	"github.com/luci/luci-go/common/api/dm/distributor/jobsim"
	"github.com/luci/luci-go/common/api/dm/service/v1"
)

type jobsimStatus string

const (
	jobsimRunnable  jobsimStatus = "runnable"
	jobsimRunning   jobsimStatus = "running"
	jobsimFailed    jobsimStatus = "failed"
	jobsimCancelled jobsimStatus = "cancelled"
	jobsimFinished  jobsimStatus = "finished"
)

type jobsimExecution struct {
	_  string `gae:"$kind,jobsim.Task"`
	ID string `gae:"$id"`

	Status        jobsimStatus `gae:",noindex"`
	StateOrReason string       `gae:",noindex"`

	ExAuth dm.Execution_Auth `gae:",noindex"`

	Calculation jobsim.Phrase `gae:",noindex"`
	CfgName     string        `gae:",noindex"`
}

func getTaskResult(status jobsimStatus, stateOrReason string) *distributor.TaskResult {
	switch status {
	case jobsimRunnable, jobsimRunning:
		return nil

	case jobsimFinished:
		return &distributor.TaskResult{
			PersistentState: distributor.PersistentState(stateOrReason)}
	}

	tr := &distributor.TaskResult{AbnormalFinish: &dm.AbnormalFinish{
		Reason: stateOrReason}}
	switch status {
	case jobsimFailed:
		tr.AbnormalFinish.Status = dm.AbnormalFinish_FAILED
	case jobsimCancelled:
		tr.AbnormalFinish.Status = dm.AbnormalFinish_CANCELLED
	}
	return tr
}

// TaskResult is the result of a Jobsim task.
type TaskResult struct {
	Success bool  `json:"success"`
	Result  int64 `json:"result,string"`
}

// ToJSON returns a JSON string encoding for this TaskResult.
func (t *TaskResult) ToJSON() string {
	ret, err := json.Marshal(t)
	if err != nil {
		panic(err)
	}
	return string(ret)
}

// TaskResultFromJSON converts a JSON string encoding to a *TaskResult.
func TaskResultFromJSON(j string) (*TaskResult, error) {
	ret := &TaskResult{}
	err := json.Unmarshal([]byte(j), ret)
	return ret, err
}
