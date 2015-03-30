// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package swarming

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/luci/luci-go/client/internal/common"
)

// TaskID is a unique reference to a Swarming task.
type TaskID string

// Swarming defines a Swarming client.
type Swarming struct {
	host   string
	client *http.Client
}

func (s *Swarming) getJSON(resource string, v interface{}) error {
	if len(resource) == 0 || resource[0] != '/' {
		return errors.New("resource must start with '/'")
	}
	status, err := common.GetJSON(s.client, s.host+resource, v)
	if status == http.StatusNotFound {
		return errors.New("not found")
	}
	return err
}

// NewSwarming returns a new Swarming client.
func NewSwarming(host string) (*Swarming, error) {
	host = strings.TrimRight(host, "/")
	return &Swarming{host, http.DefaultClient}, nil
}

// FetchRequest returns the TaskRequest.
func (s *Swarming) FetchRequest(id TaskID) (*TaskRequest, error) {
	out := &TaskRequest{}
	err := s.getJSON("/swarming/api/v1/client/task/"+string(id)+"/request", out)
	return out, err
}

// TaskRequestProperties describes the idempotent properties of a task.
type TaskRequestProperties struct {
	Commands             [][]string        `json:"commands"`
	Data                 [][]string        `json:"data"`
	Dimensions           map[string]string `json:"dimensions"`
	Env                  map[string]string `json:"env"`
	ExecutionTimeoutSecs int               `json:"execution_timeout_secs"`
	Idempotent           bool              `json:"idempotent"`
	IoTimeoutSecs        int               `json:"io_timeout_secs"`
}

// TaskRequest describes a complete request.
type TaskRequest struct {
	//"created_ts": "2014-10-24 00:00:00",
	//"expiration_ts": "2014-10-24 00:00:00",
	Name           string                `json:"name"`
	Priority       int                   `json:"priority"`
	Properties     TaskRequestProperties `json:"properties"`
	PropertiesHash string                `json:"properties_hash"`
	Tags           []string              `json:"tags"`
	User           string                `json:"user"`
}

// TaskResult describes the results of a task.
type TaskResult struct {
	TaskRequest TaskRequest `json:"request"`
	//"abandoned_ts": "2014-10-24 00:00:00",
	BotID      string `json:"bot_id"`
	BotVersion string `json:"bot_version"`
	//"completed_ts": "2014-10-24 00:00:00",
	//"created_ts": "2014-10-24 00:00:00",
	DedupedFrom     string    `json:"deduped_from"`
	Durations       []float64 `json:"durations"`
	ExitCodes       []int     `json:"exit_codes"`
	Failure         bool      `json:"failure"`
	ID              TaskID    `json:"id"`
	InternalFailure bool      `json:"internal_failure"`
	//"modified_ts": "2014-10-24 00:00:00",
	Name           string   `json:"name"`
	PropertiesHash string   `json:"properties_hash"`
	ServerVersions []string `json:"server_versions"`
	// "started_ts": "2014-10-24 00:00:00",
	State     int    `json:"state"`
	TryNumber int    `json:"try_number"`
	User      string `json:"user"`
}

// Duration returns the total duration of a task.
func (s *TaskResult) Duration() (out time.Duration) {
	for _, d := range s.Durations {
		out += time.Duration(d) * time.Second
	}
	return
}
