// Copyright 2019 The LUCI Authors.
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

package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/kr/pretty"
	"github.com/maruel/subcommands"
	"google.golang.org/api/gensupport"
	"google.golang.org/api/googleapi"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/signals"
)

func cmdQuery(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "query <options>",
		ShortDesc: "TODO",
		LongDesc:  "TODO",
		CommandRun: func() subcommands.CommandRun {
			r := &queryRun{}
			r.Init(defaultAuthOpts)
			return r
		},
	}
	// TODO: migrate comments
	// """Returns raw JSON information via an URL endpoint. Use 'query-list' to
	// gather the list of API methods from the server.

	// Examples:
	//   Raw task request and results:
	//     swarming.py query -S server-url.com task/123456/request
	//     swarming.py query -S server-url.com task/123456/result

	//   Listing all bots:
	//     swarming.py query -S server-url.com bots/list

	//   Listing last 10 tasks on a specific bot named 'bot1':
	//     swarming.py query -S server-url.com --limit 10 bot/bot1/tasks

	//   Listing last 10 tasks with tags os:Ubuntu-14.04 and pool:Chrome. Note that
	//   quoting is important!:
	//     swarming.py query -S server-url.com --limit 10 \\
	//         'tasks/list?tags=os:Ubuntu-14.04&tags=pool:Chrome'
	// """
}

type queryRun struct {
	commonFlags
	method     string
	limit      int64
	jsonOutput string
}

const defaultQueryLimit = 200

func (r *queryRun) Init(defaultAuthOpts auth.Options) {
	r.commonFlags.Init(defaultAuthOpts)

	r.Flags.Int64Var(&r.limit, "limit", defaultQueryLimit, "Limit to enforce on limitless items (like number of tasks)")
	r.Flags.StringVar(&r.jsonOutput, "json-output", "", "Path to JSON output file (otherwise prints to stdout)")
}

func (r *queryRun) Parse(args []string) error {
	if err := r.commonFlags.Parse(); err != nil {
		return err
	}

	// TODO check query
	if len(args) != 1 {
		return errors.Reason("Must specify only method name and optionally query args properly escaped.").Err()
	}
	r.method = args[0]

	return nil
}

func (r *queryRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if err := r.Parse(args); err != nil {
		printError(a, err)
		return 1
	}
	cl, err := r.defaultFlags.StartTracing()
	if err != nil {
		printError(a, err)
		return 1
	}
	defer cl.Close()

	if err := r.main(a, args, env); err != nil {
		printError(a, err)
		return 1
	}

	return 0
}

func (r *queryRun) main(a subcommands.Application, args []string, _ subcommands.Env) error {
	ctx, cancel := context.WithCancel(r.defaultFlags.MakeLoggingContext(os.Stderr))
	signals.HandleInterrupt(cancel)

	client, err := r.createAuthClient(ctx)
	if err != nil {
		return err
	}

	s, err := r.createRawSwarmingService(client, r.serverURL+swarmingAPISuffix)
	if err != nil {
		return err
	}

	return r.query(s, client, ctx)
}

func (r *queryRun) query(s *swarming.Service, hc *http.Client, ctx context.Context) error {
	c := &QueryCall{s: s, urlParams_: make(gensupport.URLParams), httpClient: hc}
	c.Context(ctx)
	c.Limit(r.limit)
	c.Method(r.method)

	res, err := c.Do()

	pretty.Println(res)

	return err
}

// TODO: move SwarmingRpcsQuery to QueryCall to swarming-gen.go
type SwarmingRpcsQuery struct {
	Cursor string `json:"cursor,omitempty"`

	Items []string `json:"items,omitempty"`

	Now string `json:"now,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Cursor") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Cursor") to include in API
	// requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

type QueryCall struct {
	s          *swarming.Service
	urlParams_ gensupport.URLParams
	ctx_       context.Context
	httpClient *http.Client
	method     string
}

func (c *QueryCall) Context(ctx context.Context) *QueryCall {
	c.ctx_ = ctx
	return c
}

func (c *QueryCall) Limit(limit int64) *QueryCall {
	c.urlParams_.Set("limit", fmt.Sprint(limit))
	return c
}

func (c *QueryCall) Method(method string) *QueryCall {
	c.method = method
	return c
}

func (c *QueryCall) Do(opts ...googleapi.CallOption) (*SwarmingRpcsQuery, error) {
	gensupport.SetOptions(c.urlParams_, opts...)
	res, err := c.doRequest(c.method, "json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, &googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		}
	}
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	ret := &SwarmingRpcsQuery{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := gensupport.DecodeResponse(target, res); err != nil {
		return nil, err
	}

	return ret, nil
}

func (c *QueryCall) doRequest(method, alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	reqHeaders.Set("User-Agent", googleapi.UserAgent)
	var body io.Reader = nil
	c.urlParams_.Set("alt", alt)
	c.urlParams_.Set("prettyPrint", "false")
	urls := googleapi.ResolveRelative(c.s.BasePath, method)
	urls += "?" + c.urlParams_.Encode()
	req, err := http.NewRequest("GET", urls, body)
	if err != nil {
		return nil, err
	}
	req.Header = reqHeaders
	return gensupport.SendRequest(c.ctx_, c.httpClient, req)
}
