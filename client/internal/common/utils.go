// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package common

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"runtime"
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

// IsDirectory returns true if path is a directory and is accessible.
func IsDirectory(path string) bool {
	fileInfo, err := os.Stat(path)
	return err == nil && fileInfo.IsDir()
}

func IsWindows() bool {
	return runtime.GOOS == "windows"
}

// StringsCollect accumulates string values from repeated flags.
// Use with flag.Var to accumlate values from "-flag s1 -flag s2".
type StringsCollect struct {
	Values *[]string
}

func (c *StringsCollect) String() string {
	return strings.Join(*c.Values, " ")
}

func (c *StringsCollect) Set(value string) error {
	*c.Values = append(*c.Values, value)
	return nil
}

// NKVArgCollect accumulates multiple key-value for a given flag.
// The only supported form is --flag key=value .
// If the same key appears several times, the value of last occurence is used.
type NKVArgCollect struct {
	Values  *KeyValVars
	OptName string
}

func (c *NKVArgCollect) SetAsFlag(flags *flag.FlagSet, values *KeyValVars, name string, usage string) {
	c.Values = values
	c.OptName = name
	flags.Var(c, name, usage)
}

func (c *NKVArgCollect) String() string {
	return pretty.Sprintf("%v", *c.Values)
}

func (c *NKVArgCollect) Set(value string) error {
	kv := strings.SplitN(value, "=", 2)
	if len(kv) != 2 {
		return fmt.Errorf("please use %s FOO=BAR", c.OptName)
	}
	key, value := kv[0], kv[1]
	// TODO(tandrii): decode value as utf-8.
	(*c.Values)[key] = value
	return nil
}
