// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package settings

import (
	"fmt"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/luci_config/server/cfgclient"
	"github.com/luci/luci-go/luci_config/server/cfgclient/textproto"
	milocfg "github.com/luci/luci-go/milo/common/config"
	"github.com/luci/luci-go/server/router"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
)

// Project is a LUCI project.
type Project struct {
	// The ID of the project, as per self defined.  This is not the luci-cfg
	// name.
	ID string `gae:"$id"`
	// The luci-cfg name of the project.
	Name string
	// The Project data in protobuf binary format.
	Data []byte `gae:",noindex"`
}

// UpdateHandler is an HTTP handler that handles configuration update requests.
func UpdateHandler(ctx *router.Context) {
	c, h := ctx.Context, ctx.Writer
	err := Update(c)
	if err != nil {
		logging.WithError(err).Errorf(c, "Update Handler encountered error")
		h.WriteHeader(500)
	}
	logging.Infof(c, "Successfully completed")
	h.WriteHeader(200)
}

// Update internal configuration based off luci-cfg.
// update updates Milo's configuration based off luci config.  This includes
// scanning through all project and extract all console configs.
func Update(c context.Context) error {
	cfgName := info.AppID(c) + ".cfg"

	var (
		configs []*milocfg.Project
		metas   []*cfgclient.Meta
	)
	if err := cfgclient.Projects(c, cfgclient.AsService, cfgName, textproto.Slice(&configs), &metas); err != nil {
		logging.WithError(err).Errorf(c, "Encountered error while getting project config for %s", cfgName)
		return err
	}

	// A map of project ID to project.
	projects := map[string]*Project{}
	for i, proj := range configs {
		projectName, _, _ := metas[i].ConfigSet.SplitProject()

		logging.Infof(c, "Prossing %s", projectName)
		if dup, ok := projects[proj.ID]; ok {
			return fmt.Errorf(
				"Duplicate project ID: %s. (%s and %s)", proj.ID, dup.Name, projectName)
		}
		p := &Project{
			ID:   proj.ID,
			Name: string(projectName),
		}
		projects[proj.ID] = p

		var err error
		p.Data, err = proto.Marshal(proj)
		if err != nil {
			return err
		}
	}

	// Now load all the data into the datastore.
	projs := make([]*Project, 0, len(projects))
	for _, proj := range projects {
		projs = append(projs, proj)
	}
	if err := ds.Put(c, projs); err != nil {
		return err
	}

	// Delete entries that no longer exist.
	q := ds.NewQuery("Project").KeysOnly(true)
	allProjs := []Project{}
	ds.GetAll(c, q, &allProjs)
	toDelete := []Project{}
	for _, proj := range allProjs {
		if _, ok := projects[proj.ID]; !ok {
			toDelete = append(toDelete, proj)
		}
	}
	ds.Delete(c, toDelete)

	return nil
}

// GetAllProjects returns all registered projects.
func GetAllProjects(c context.Context) ([]*milocfg.Project, error) {
	q := ds.NewQuery("Project")
	q.Order("ID")

	ps := []*Project{}
	err := ds.GetAll(c, q, &ps)
	if err != nil {
		return nil, err
	}
	results := make([]*milocfg.Project, len(ps))
	for i, p := range ps {
		results[i] = &milocfg.Project{}
		if err := proto.Unmarshal(p.Data, results[i]); err != nil {
			return nil, err
		}
	}
	return results, nil
}

// GetProject returns the requested project.
func GetProject(c context.Context, projName string) (*milocfg.Project, error) {
	// Next, Try datastore
	p := Project{ID: projName}
	if err := ds.Get(c, &p); err != nil {
		return nil, err
	}
	mp := milocfg.Project{}
	if err := proto.Unmarshal(p.Data, &mp); err != nil {
		return nil, err
	}

	return &mp, nil
}

// GetConsole returns the requested console instance.
func GetConsole(c context.Context, projName, consoleName string) (*milocfg.Console, error) {
	p, err := GetProject(c, projName)
	if err != nil {
		return nil, err
	}
	for _, cs := range p.Consoles {
		if cs.Name == consoleName {
			return cs, nil
		}
	}
	return nil, fmt.Errorf("Console %s not found in project %s", consoleName, projName)
}
