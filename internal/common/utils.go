// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package common

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/kr/pretty"
)

// URLToHTTPS ensures the url is https://.
func URLToHTTPS(s string) (string, error) {
	u, err := url.Parse(s)
	if err != nil {
		return "", err
	}
	if u.Scheme != "" && u.Scheme != "https" {
		return "", errors.New("Only https:// scheme is accepted. It can be omitted.")
	}
	if !strings.HasPrefix(s, "https://") {
		s = "https://" + s
	}
	if _, err = url.Parse(s); err != nil {
		return "", err
	}
	return s, nil
}

func IsDirectory(path string) (bool, error) {
	fileInfo, err := os.Stat(path)
	return fileInfo.IsDir(), err
}

// implements flags.Value interface
// usage:
// flags.Var(
type NStringsCollect struct {
	Values *[]string
}

func (c *NStringsCollect) String() string {
	return strings.Join(*c.Values, " ")
}

func (c *NStringsCollect) Set(value string) error {
	*c.Values = append(*c.Values, value)
	return nil
}

type NKVArgCollect struct {
	Values    *[][]string
	OptName   string
	lastChunk string
}

func NewNKVArgCollect(values *[][]string, optionName string) NKVArgCollect {
	return NKVArgCollect{values, optionName, ""}
}

func (c *NKVArgCollect) GetPostParseError() error {
	if c.lastChunk == "" {
		return nil
	}
	return fmt.Errorf("Please use %s FOO=BAR or %s FOO BAR", c.OptName, c.OptName)
}

func (c *NKVArgCollect) String() string {
	return pretty.Sprintf("%v", *c.Values)
}

func (c *NKVArgCollect) Set(value string) error {
	if strings.Contains(value, "=") {
		kv := strings.Split(value, "=")
		if len(kv) != 2 {
			return fmt.Errorf("Please use %s FOO=BAR or %s FOO BAR", c.OptName, c.OptName)
		}
		*c.Values = append(*c.Values, kv)
	} else {
		if c.lastChunk == "" {
			c.lastChunk = value
		} else {
			*c.Values = append(*c.Values, []string{c.lastChunk, value})
		}
	}
	return nil
}
