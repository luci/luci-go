// Copyright 2018 The LUCI Authors.
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

package iam

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"encoding/json"

	. "github.com/smartystreets/goconvey/convey"
)

func TestServiceAccounts(t *testing.T) {
	Convey("CreateServiceAccount works", t, func(c C) {
		projectId := "abcdefg"
		urlPath := fmt.Sprintf("/v1/projects/%s/serviceAccounts", projectId)
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}

			switch r.URL.Path {
			case urlPath:
				body, err := ioutil.ReadAll(r.Body)
				if err != nil {
					panic(err)
				}
				var request struct {
					AccountId      string         `json:"accountId"`
					ServiceAccount ServiceAccount `json:"serviceAccount"`
				}
				err = json.Unmarshal(body, &request)
				if err != nil {
					panic(err)
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(200)

				request.ServiceAccount.ProjectId = projectId
				request.ServiceAccount.DisplayName = request.AccountId
				resp, err := json.Marshal(&request.ServiceAccount)
				if err != nil {
					panic(err)
				}
				w.Write([]byte(resp))

			default:
				panic(fmt.Errorf("Unknown URL: %q\n", r.URL.Path))
			}
		}))
		defer ts.Close()

		cl := Client{
			Client:   http.DefaultClient,
			BasePath: ts.URL,
		}

		accountId := "newAccount123"
		expected := &ServiceAccount{
			ProjectId:   projectId,
			DisplayName: accountId,
		}
		serviceAccount, err := cl.CreateServiceAccount(context.Background(), projectId, accountId, accountId)
		So(err, ShouldBeNil)
		So(serviceAccount, ShouldResemble, expected)
	})
}
