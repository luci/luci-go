// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package config

import (
	"net/url"

	config "github.com/luci/luci-go/common/config"
	"golang.org/x/net/context"
)

// Use adds an implementation of the config service which stores the
// results on the local filesystem.
// NOT SUITABLE FOR PRODUCTION USE.
func Use(c context.Context) context.Context {
	return config.SetFactory(c, func(ic context.Context) config.Interface {
		return &filesystemImpl{}
	})
}

type filesystemImpl struct {
}

// TODO(martiniss) implement a file system config provider

func (fs *filesystemImpl) GetConfig(configSet, path string) (*config.Config, error) {
	panic("UNIMPLEMENTED")
}

func (fs *filesystemImpl) GetConfigByHash(contentHash string) (string, error) {
	panic("UNIMPLEMENTED")
}

func (fs *filesystemImpl) GetConfigSetLocation(configSet string) (*url.URL, error) {
	panic("UNIMPLEMENTED")
}

func (fs *filesystemImpl) GetProjectConfigs(path string) ([]*config.Config, error) {
	panic("UNIMPLEMENTED")
}

func (fs *filesystemImpl) GetProjects() ([]*config.Project, error) {
	panic("UNIMPLEMENTED")
}

func (fs *filesystemImpl) GetRefConfigs(path string) ([]*config.Config, error) {
	panic("UNIMPLEMENTED")
}

func (fs *filesystemImpl) GetRefs(projectID string) ([]string, error) {
	panic("UNIMPLEMENTED")
}
