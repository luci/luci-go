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

package distributor

import (
	"fmt"

	"go.chromium.org/luci/dm/api/service/v1"
	"go.chromium.org/luci/tumble"
	"golang.org/x/net/context"
)

type TestFactoryFn func(context.Context, *Config) D

type TestFactoryMap map[string]TestFactoryFn

type testRegistry struct {
	finishExecutionImpl FinishExecutionFn
	data                TestFactoryMap
}

var _ Registry = (*testRegistry)(nil)

// NewTestingRegistry returns a new testing registry.
//
// The mocks dictionary maps from cfgName to a mock implementation of the
// distributor.
func NewTestingRegistry(mocks TestFactoryMap, fFn FinishExecutionFn) Registry {
	return &testRegistry{fFn, mocks}
}

func (t *testRegistry) FinishExecution(c context.Context, eid *dm.Execution_ID, rslt *dm.Result) ([]tumble.Mutation, error) {
	return t.finishExecutionImpl(c, eid, rslt)
}

func (t *testRegistry) MakeDistributor(c context.Context, cfgName string) (D, string, error) {
	ret, ok := t.data[cfgName]
	if !ok {
		return nil, "", fmt.Errorf("unknown distributor configuration: %q", cfgName)
	}
	return ret(c, &Config{
		DMHost:  "test-dm-host.example.com",
		Version: "test-version",
		Name:    cfgName,
	}), "testing", nil
}
