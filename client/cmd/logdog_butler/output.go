// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"github.com/luci/luci-go/client/internal/logdog/butler/output"
	"github.com/luci/luci-go/common/flag/multiflag"
)

type outputFactory interface {
	option() multiflag.Option
	configOutput(a *application) (output.Output, error)
}

// outputConfigFlag instance that produces a MessageOutput instance when run.
type outputConfigFlag struct {
	multiflag.MultiFlag
}

// Adds an output factory to this outputConfigFlag instance.
func (ocf *outputConfigFlag) AddFactory(f outputFactory) {
	ocf.Options = append(ocf.Options, f.option())
}

// Returns the Factory associated with the configured flag.
func (ocf *outputConfigFlag) getFactory() outputFactory {
	if ocf.Selected == nil {
		return nil
	}

	if o, ok := ocf.Selected.(*outputOption); ok {
		return o.factory
	}
	return nil
}

// outputOption is a multiflag.Option extension that records its Factory when
// chosen.
type outputOption struct {
	multiflag.FlagOption
	factory outputFactory
}

// newOutputOption instantiates a new outputOption.
func newOutputOption(name, description string, f outputFactory) *outputOption {
	return &outputOption{
		FlagOption: multiflag.FlagOption{
			Name:        name,
			Description: description,
		},
		factory: f,
	}
}

// Global store of registered output options. This will be
// conditionally-compiled based on build tags.
var outputFactories = []outputFactory{
	&logOutputFactory{},
}

// Registers a new output option. This is meant to be called by 'init()' methods
// of each option.
func registerOutputFactory(f outputFactory) {
	outputFactories = append(outputFactories, f)
}

// Returns a slice of registered output options.
func getOutputFactories() []outputFactory {
	return outputFactories
}
