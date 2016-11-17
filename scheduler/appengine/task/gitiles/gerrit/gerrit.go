// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package gerrit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/gae/service/urlfetch"
)

// GitTime is a date and time representation used by git log.
type GitTime time.Time

// UnmarshalJSON parses a date and time from the input buffer.
func (gt GitTime) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	// Time stamps from Gitiles sometimes have a UTC offset (e.g., -7800), and
	// sometimes not, so we try both variants before failing.
	t, err := time.Parse(`Mon Jan 02 15:04:05 2006`, s)
	switch err.(type) {
	case *time.ParseError:
		t, err = time.Parse(`Mon Jan 02 15:04:05 2006 -0700`, s)
	}
	if err == nil {
		gt = GitTime(t)
	}
	return err
}

// Commit represents a single entry in the log returned by Gitiles REST API.
type Commit struct {
	Commit string `json:"commit"`
	Author struct {
		Name  string  `json:"name"`
		Email string  `json:"email"`
		Time  GitTime `json:"time"`
	}
	Committer struct {
		Name  string  `json:"name"`
		Email string  `json:"email"`
		Time  GitTime `json:"time"`
	}
	Message string `json:"message"`
}

// Log is therepresentation of Git log as a sequence of commits.
type Log []Commit

// Len returns the length of the Git log.
func (l Log) Len() int {
	return len(l)
}

// Swap swaps the two elements.
func (l Log) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}

// Less reutnr true if the element i is smaller than j.
func (l Log) Less(i, j int) bool {
	return time.Time(l[i].Committer.Time).Before(time.Time(l[j].Committer.Time))
}

// Gerrit exposes the Gerrit REST API.
type Gerrit struct {
	Host *url.URL
}

// NewGerrit returns the new Gerrit instance for the given URL.
func NewGerrit(host *url.URL) *Gerrit {
	return &Gerrit{
		Host: host,
	}
}

// StatusError represents an HTTP status error.
type StatusError struct {
	Code int
}

// Error returns the string representation of the HTTP status error.
func (e StatusError) Error() string {
	return fmt.Sprintf("HTTP request failed: %v", http.StatusText(e.Code))
}

// IsNotFound return true if the error is the 404 Status Not Found.
func IsNotFound(err error) bool {
	switch err := err.(type) {
	case *StatusError:
		return err.Code == http.StatusNotFound
	default:
		return false
	}
}

// Request can be used to make a Gerrit REST API request.
func (g *Gerrit) Request(c context.Context, method, route, query string) ([]byte, error) {
	u, err := url.Parse(g.Host.String())
	if err != nil {
		return nil, err
	}

	u.Path = path.Join(u.Path, route)
	u.RawQuery = query

	req, err := http.NewRequest(method, u.String(), nil)
	if err != nil {
		return nil, err
	}
	client := http.Client{Transport: urlfetch.Get(c)}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, StatusError{res.StatusCode}
	}

	r := bufio.NewReader(res.Body)

	// The first line of the input is the XSSI guard ")]}'".
	if _, err := r.ReadSlice('\n'); err != nil {
		return nil, err
	}

	return ioutil.ReadAll(r)
}

// GetRefs returns HEAD revision for each ref specified in refs.
func (g *Gerrit) GetRefs(c context.Context, refs []string) (map[string]string, error) {
	v := url.Values{}
	v.Set("format", "JSON")

	b, err := g.Request(c, "GET", "+refs", v.Encode())
	if err != nil {
		return nil, err
	}

	var values map[string]struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(b, &values); err != nil {
		return nil, err
	}

	result := map[string]string{}
	for k, v := range values {
		for _, ref := range refs {
			if ref == k {
				result[ref] = v.Value
			}
		}
	}

	return result, nil
}

// GetLog returns log for a given ref since a given commit.
func (g *Gerrit) GetLog(c context.Context, ref, since string) (Log, error) {
	var result []Commit
	next := ref

	v := url.Values{}
	v.Set("format", "JSON")
	v.Set("n", strconv.Itoa(100))

	for next != "" {
		b, err := g.Request(c, "GET", fmt.Sprintf("+log/%s..%s", since, next), v.Encode())
		if err != nil {
			return nil, err
		}

		var values struct {
			Log  []Commit `json:"log"`
			Next string   `json:"next,omitempty"`
		}
		if err := json.Unmarshal(b, &values); err != nil {
			return nil, err
		}
		result = append(result, values.Log...)
		next = values.Next
	}

	return result, nil
}
