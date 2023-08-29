// Copyright 2023 The LUCI Authors.
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

package rules

import (
	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/common/data/stringset"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/config_service/internal/common"
)

func validateProjectsCfg(vctx *validation.Context, configSet, path string, content []byte) error {
	vctx.SetFile(path)
	cfg := &cfgcommonpb.ProjectsCfg{}
	if err := prototext.Unmarshal(content, cfg); err != nil {
		vctx.Errorf("invalid projects proto: %s", err)
		return nil
	}
	seenTeams := stringset.New(len(cfg.GetTeams()))
	for i, team := range cfg.GetTeams() {
		vctx.Enter("teams #%d", i)
		vctx.Enter("name")
		validateUniqueID(vctx, team.GetName(), seenTeams, nil)
		vctx.Exit()
		if len(team.GetMaintenanceContact()) == 0 {
			vctx.Errorf("maintenance_contact is required")
		}
		for i, contact := range team.GetMaintenanceContact() {
			vctx.Enter("maintenance_contact #%d", i)
			validateEmail(vctx, contact)
			vctx.Exit()
		}
		if len(team.GetEscalationContact()) == 0 {
			vctx.Warningf("escalation_contact is recommended")
		}
		for i, contact := range team.GetEscalationContact() {
			vctx.Enter("escalation_contact #%d", i)
			validateEmail(vctx, contact)
			vctx.Exit()
		}
		vctx.Exit()
	}
	validateSorted[*cfgcommonpb.Team](vctx, cfg.GetTeams(), "teams", func(t *cfgcommonpb.Team) string {
		return t.GetName()
	})

	seenProjects := stringset.New(len(cfg.GetProjects()))
	for i, project := range cfg.GetProjects() {
		vctx.Enter("projects #%d", i)
		vctx.Enter("id")
		validateUniqueID(vctx, project.GetId(), seenProjects, func(vctx *validation.Context, id string) {
			if _, err := config.ProjectSet(id); err != nil {
				vctx.Errorf("invalid id: %s", err)
			}
		})
		vctx.Exit()

		vctx.Enter("gitiles_location")
		if err := common.ValidateGitilesLocation(project.GetGitilesLocation()); err != nil {
			vctx.Error(err)
		}
		vctx.Exit()

		vctx.Enter("owned_by")
		switch ownedBy := project.GetOwnedBy(); {
		case ownedBy == "":
			vctx.Errorf("not specified")
		case !seenTeams.Has(ownedBy):
			vctx.Errorf(`unknown team %q`, ownedBy)
		}
		vctx.Exit()

		vctx.Exit()
	}
	validateSorted[*cfgcommonpb.Project](vctx, cfg.GetProjects(), "projects", func(project *cfgcommonpb.Project) string {
		return project.GetId()
	})
	return nil
}

func validateProjectMetadata(vctx *validation.Context, configSet, path string, content []byte) error {
	vctx.SetFile(path)
	cfg := &cfgcommonpb.ProjectCfg{}
	if err := prototext.Unmarshal(content, cfg); err != nil {
		vctx.Errorf("invalid project proto: %s", err)
		return nil
	}
	if cfg.GetName() == "" {
		vctx.Errorf("name is not specified")
	}
	for i, access := range cfg.GetAccess() {
		vctx.Enter("access #%d", i)
		validateAccess(vctx, access)
		vctx.Exit()
	}
	return nil
}
