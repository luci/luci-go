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

package buildbot

import (
	"bytes"
	"encoding/json"
	"fmt"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/milo/common/model"
)

type Result int

//go:generate stringer -type Result

const (
	NoResult Result = iota - 1
	Success
	Warning
	Failure
	Skipped
	Exception
	Retry
	resultEnd
)

// Status converts r into a model.Status.
func (r Result) Status() model.Status {
	switch r {
	case NoResult:
		return model.Running
	case Success:
		return model.Success
	case Warning:
		return model.Warning
	case Failure:
		return model.Failure
	case Skipped:
		return model.NotRun
	case Exception, Retry:
		return model.Exception
	default:
		panic(fmt.Errorf("unknown status %d", r))
	}
}

func (r *Result) MarshalJSON() ([]byte, error) {
	var buildbotFormat *int
	if *r != NoResult {
		v := int(*r)
		buildbotFormat = &v
	}
	return json.Marshal(buildbotFormat)
}

func (r *Result) UnmarshalJSON(data []byte) error {
	var buildbotFormat *int
	if err := json.Unmarshal(data, &buildbotFormat); err != nil {
		return err
	}

	if buildbotFormat == nil {
		*r = NoResult
	} else {
		*r = Result(*buildbotFormat)
	}
	return nil
}

type StepResults struct {
	Result
	rest []interface{} // part of JSON we don't understand
}

func (r *StepResults) MarshalJSON() ([]byte, error) {
	buildbotFormat := append([]interface{}{nil}, r.rest...)
	if r.Result != NoResult {
		buildbotFormat[0] = r.Result
	}
	return json.Marshal(buildbotFormat)
}

func (r *StepResults) UnmarshalJSON(data []byte) error {
	var buildbotFormat []interface{}
	m := json.NewDecoder(bytes.NewReader(data))
	m.UseNumber()
	if err := m.Decode(&buildbotFormat); err != nil {
		return err
	}

	buf := StepResults{Result: NoResult}
	if len(buildbotFormat) > 0 {
		if buildbotFormat[0] != nil {
			n, ok := buildbotFormat[0].(json.Number)
			if !ok {
				return errors.Reason("expected a number, received %T", buildbotFormat[0]).Err()
			}
			v, err := n.Int64()
			if err != nil {
				return err
			}
			buf.Result = Result(int(v))
			if buf.Result < 0 || buf.Result >= resultEnd {
				return errors.Reason("invalid result %d", v).Err()
			}
		}
		if len(buildbotFormat) > 1 {
			buf.rest = buildbotFormat[1:]
			// otherwise keep it nil, so we can compare StepResults structs
			// in tests
		}
	}
	*r = buf
	return nil
}
