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

package gitiles

import (
	"context"
)

type MockedGitilesClient struct {
	Data map[string]string // Data for MockedGitilesClient to return
}

func (cl *MockedGitilesClient) sendRequest(c context.Context, url string, params map[string]string) (string, error) {
	if val, ok := cl.Data[url]; ok {
		return val, nil
	}
	return `{"logs":[]}`, nil
}

func MockedGitilesClientContext(c context.Context, data map[string]string) context.Context {
	return context.WithValue(c, MockedGitilesClientKey, &MockedGitilesClient{
		Data: data,
	})
}
