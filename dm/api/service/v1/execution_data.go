// Copyright 2016 The LUCI Authors.
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

package dm

import "errors"

// NewExecutionScheduling creates an Execution in the SCHEDULING state.
func NewExecutionScheduling() *Execution {
	return &Execution{
		Data: &Execution_Data{
			ExecutionType: &Execution_Data_Scheduling_{
				&Execution_Data_Scheduling{}}}}
}

// NewExecutionRunning creates an Execution in the RUNNING state.
func NewExecutionRunning() *Execution {
	return &Execution{
		Data: &Execution_Data{
			ExecutionType: &Execution_Data_Running_{
				&Execution_Data_Running{}}}}
}

// NewExecutionStopping creates an Execution in the STOPPING state.
func NewExecutionStopping() *Execution {
	return &Execution{
		Data: &Execution_Data{
			ExecutionType: &Execution_Data_Stopping_{
				&Execution_Data_Stopping{}}}}
}

// NewExecutionFinished creates an Execution in the FINISHED state.
func NewExecutionFinished(result *JsonResult) *Execution {
	return &Execution{
		Data: &Execution_Data{
			ExecutionType: &Execution_Data_Finished_{
				&Execution_Data_Finished{Data: result}}}}
}

// NewExecutionAbnormalFinish creates an Execution in the ABNORMAL_FINISH state.
func NewExecutionAbnormalFinish(af *AbnormalFinish) *Execution {
	return &Execution{
		Data: &Execution_Data{
			ExecutionType: &Execution_Data_AbnormalFinish{af}}}
}

// State computes the Execution_State for the current Execution_Data
func (d *Execution_Data) State() Execution_State {
	if d != nil {
		switch d.ExecutionType.(type) {
		case *Execution_Data_Scheduling_:
			return Execution_SCHEDULING
		case *Execution_Data_Running_:
			return Execution_RUNNING
		case *Execution_Data_Stopping_:
			return Execution_STOPPING
		case *Execution_Data_Finished_:
			return Execution_FINISHED
		case *Execution_Data_AbnormalFinish:
			return Execution_ABNORMAL_FINISHED
		}
	}
	return Execution_SCHEDULING
}

// Normalize returns an error iff the Execution_Auth has invalid form (e.g.
// contains nils).
func (a *Execution_Auth) Normalize() error {
	if a == nil {
		return errors.New("Execution_Auth is nil")
	}
	if a.Id == nil {
		return errors.New("Execution_Auth.Id is nil")
	}
	return nil
}
