// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"os"

	"github.com/luci/luci-go/common/config"
)

var errNotImplemented = errors.New("not implemented")

// fileConfig is an implementation of config.Interface that represents itself
// as a configuration instance with one specific ConfigSet containing one
// specific file.
//
// This is a partial implementation of the config.Interface interface sufficient
// for a ConfigManager to use.
type fileConfig struct {
	path string

	configSet  string
	configPath string
}

var _ config.Interface = (*fileConfig)(nil)

func (fc *fileConfig) GetConfig(configSet, path string, hashOnly bool) (*config.Config, error) {
	if configSet != fc.configSet || path != fc.configPath {
		return nil, config.ErrNoConfig
	}

	buf := bytes.Buffer{}
	fd, err := os.Open(fc.path)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	_, err = buf.ReadFrom(fd)
	if err != nil {
		return nil, err
	}

	hash := sha256.Sum256(buf.Bytes())
	return &config.Config{
		Content:     buf.String(),
		ContentHash: hex.EncodeToString(hash[:]),
	}, nil
}

func (fc *fileConfig) GetConfigByHash(contentHash string) (string, error) {
	return "", errNotImplemented
}

func (fc *fileConfig) GetConfigSetLocation(configSet string) (*url.URL, error) {
	return nil, errNotImplemented
}

func (fc *fileConfig) GetProjectConfigs(path string, hashesOnly bool) ([]config.Config, error) {
	return nil, errNotImplemented
}

func (fc *fileConfig) GetProjects() ([]config.Project, error) {
	return nil, errNotImplemented
}

func (fc *fileConfig) GetRefConfigs(path string, hashesOnly bool) ([]config.Config, error) {
	return nil, errNotImplemented
}

func (fc *fileConfig) GetRefs(projectID string) ([]string, error) {
	return nil, errNotImplemented
}
