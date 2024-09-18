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

package protowalk

import (
	"fmt"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/reflectutil"
)

// ResultData is the data produced from the FieldProcessor.
type ResultData struct {
	// Message is a human-readable message from FieldProcessor.Process describing
	// the nature of the violation or correction (depends on the FieldProcessor
	// implementation).
	Message string

	// IsErr is a flag for if the processed result should be considered as an error.
	IsErr bool
}

func (d ResultData) String() string {
	return d.Message
}

// Result is a singluar outcome from a FieldProcessor.Process which acted on
// a message field.
type Result struct {
	// Path is the path to the field where the FieldProcessor acted.
	Path reflectutil.Path

	Data ResultData
}

func (r Result) String() string {
	return fmt.Sprintf("%s: %s", r.Path, r.Data)
}

func (r Result) Err() error {
	if r.Data.IsErr {
		return errors.New(r.String())
	}
	return nil
}

// Results holds a slice of Result structs for each FieldProcessor passed in.
type Results [][]Result

// Strings returns a flat list of strings for all processors.
func (r Results) Strings() []string {
	num := 0
	for _, v := range r {
		num += len(v)
	}

	if num == 0 {
		return nil
	}

	ret := make([]string, 0, num)

	for _, v := range r {
		for _, rslt := range v {
			ret = append(ret, rslt.String())
		}
	}

	return ret
}

// Err returns the error for all processors.
func (r Results) Err() error {
	merr := make(errors.MultiError, 0)
	for _, v := range r {
		for _, rslt := range v {
			merr.MaybeAdd(rslt.Err())
		}
	}
	return merr.AsError()
}
