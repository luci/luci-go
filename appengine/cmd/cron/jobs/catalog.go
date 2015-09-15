// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package jobs implements a part that talks to luci-config service to fetch
// job definitions and knows what they mean. It knows how to launch jobs and
// how to wait form them to finish.
package jobs

import (
	"fmt"

	"golang.org/x/net/context"
)

// Catalog knows how to enumerate all cron job configs across all projects.
// A method returns errors.Transient if the error is non-fatal and the call
// should be retried later. Any other error means that retry won't help.
type Catalog interface {
	// GetAllProjects returns a list of all known project ids.
	GetAllProjects(c context.Context) ([]string, error)

	// GetProjectJobs returns a list of cron jobs defined within a project or
	// empty list if no such project.
	GetProjectJobs(c context.Context, projectID string) ([]Definition, error)
}

// Definition wraps serialized versioned definition of a cron job fetch from
// the config. If revision doesn't change between two calls, the definition
// itself also doesn't change. The opposite is not true: a job MAY stay
// the same even if its revision changes.
type Definition interface {
	// JobID returns globally unique job identifier: "<ProjectID>/<JobName>".
	JobID() string

	// ProjectID returns project ID the job belongs to.
	ProjectID() string

	// Revision returns a non empty string to use to detect changes in definitions
	// without fully unpacking the job. Only equality comparisons are meaningful
	// for revision strings.
	Revision() string

	// Schedule returns cron job schedule in regular cron expression format.
	Schedule() string

	// Work return serialized representation of cron job work item.
	Work() []byte
}

// NewCatalog returns implementation of Catalog.
func NewCatalog() Catalog {
	return &fakeCatalog{}
}

// Use fake until luci-config client is available.

type fakeCatalog struct{}

type fakeDefinition struct {
	id       string
	project  string
	rev      string
	schedule string
	work     []byte
}

func (d *fakeDefinition) JobID() string     { return d.id }
func (d *fakeDefinition) ProjectID() string { return d.project }
func (d *fakeDefinition) Revision() string  { return d.rev }
func (d *fakeDefinition) Schedule() string  { return d.schedule }
func (d *fakeDefinition) Work() []byte      { return d.work }

func (cat *fakeCatalog) GetAllProjects(c context.Context) ([]string, error) {
	return []string{"project1", "project2"}, nil
}

func (cat *fakeCatalog) GetProjectJobs(c context.Context, projectID string) ([]Definition, error) {
	defs := []Definition{}
	for i := 0; i < 10; i++ {
		defs = append(defs, &fakeDefinition{
			id:       fmt.Sprintf("%s/job-%d", projectID, i),
			project:  projectID,
			rev:      "rev",
			schedule: fmt.Sprintf("*/%d * * * * * *", 10+i),
		})
	}
	return defs, nil
}
