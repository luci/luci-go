// Copyright 2018 The LUCI Authors.
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

package cipd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cipd/client/cipd/ui"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/cipd/common/cipderr"
)

// Helper structs and implementation guts of EnsurePackages call.

// ActionKind defines what is being done to a particular package.
type ActionKind string

const (
	ActionRemove  ActionKind = "remove"  // package removal
	ActionRelink  ActionKind = "relink"  // restoration of broken symlinks
	ActionRepair  ActionKind = "repair"  // restoration of deleted files
	ActionInstall ActionKind = "install" // installation or upgrade
)

// Actions lists what the cipd.Client should do or did to ensure the state of
// some single subdirectory under the installation root.
//
// Is it part of a per-directory ActionMap returned by EnsurePackages.
type Actions struct {
	ToInstall common.PinSlice `json:"to_install,omitempty"` // pins to be installed
	ToUpdate  []UpdatedPin    `json:"to_update,omitempty"`  // pins to be replaced
	ToRemove  common.PinSlice `json:"to_remove,omitempty"`  // pins to be removed
	ToRepair  []BrokenPin     `json:"to_repair,omitempty"`  // pins to be repaired

	Errors []ActionError `json:"errors,omitempty"` // all individual errors
}

// Empty is true if there are no actions specified.
func (a *Actions) Empty() bool {
	return len(a.ToInstall) == 0 &&
		len(a.ToUpdate) == 0 &&
		len(a.ToRemove) == 0 &&
		len(a.ToRepair) == 0
}

// UpdatedPin specifies a pair of pins: old and new version of a package.
type UpdatedPin struct {
	From common.Pin `json:"from"`
	To   common.Pin `json:"to"`
}

// BrokenPin specifies a pin that should be repaired and how.
type BrokenPin struct {
	Pin        common.Pin `json:"pin"`
	RepairPlan RepairPlan `json:"repair_plan"`
}

// RepairPlan describes what should be redeployed to fix a broken pin.
type RepairPlan struct {
	// NeedsReinstall is true if the package is broken to the point it is simpler
	// to completely reinstall it.
	//
	// ReinstallReason contains explanation why the reinstall is needed.
	//
	// If NeedsReinstall is false, then the package may be repaired just by
	// extracting a bunch of files, specified in ToRedeploy list.
	NeedsReinstall bool `json:"needs_reinstall,omitempty"`

	// ReinstallReason is a human-readable reason of why the package should be
	// completely reinstalled rather than selectively repaired.
	ReinstallReason string `json:"reinstall_reason,omitempty"`

	// ToRedeploy is a list of slash-separated file names (as they are specified
	// inside the package file) that needs to be reextracted from the original
	// package and relinked into the site root in order to repair the deployment.
	//
	// If this list is not empty, it means we'll need an original package file
	// to repair the deployment.
	//
	// Set only if NeedsReinstall is false.
	ToRedeploy []string `json:"to_redeploy,omitempty"`

	// ToRelink is a list of slash-separated file names (as they are specified
	// inside the package file) that needs to be relinked into the site root in
	// order to repair the deployment.
	//
	// They are already present in the .cipd/* guts, so there's no need to fetch
	// the original package file to get them.
	//
	// Set only if NeedsReinstall is false.
	ToRelink []string `json:"to_relink,omitempty"`
}

// NumBrokenFiles returns number of files that will be repaired.
func (p *RepairPlan) NumBrokenFiles() int {
	return len(p.ToRedeploy) + len(p.ToRelink)
}

// ActionError holds an error that happened when working on the pin.
type ActionError struct {
	Action       ActionKind       `json:"action"`
	Pin          common.Pin       `json:"pin"`
	Error        JSONError        `json:"error,omitempty"`         // human readable message
	ErrorCode    cipderr.Code     `json:"error_code,omitempty"`    // enum with categories
	ErrorDetails *cipderr.Details `json:"error_details,omitempty"` // structured details
}

// ActionMap is a map of subdir to the Actions which will occur within it.
type ActionMap map[string]*Actions

// Empty is true if ActionMap is empty.
func (am ActionMap) Empty() bool {
	for _, a := range am {
		if !a.Empty() {
			return false
		}
	}
	return true
}

// loopOrdered loops over the ActionMap in sorted order (by subdir).
func (am ActionMap) loopOrdered(cb func(subdir string, actions *Actions)) {
	subdirs := make(sort.StringSlice, 0, len(am))
	for subdir := range am {
		subdirs = append(subdirs, subdir)
	}
	subdirs.Sort()
	for _, subdir := range subdirs {
		cb(subdir, am[subdir])
	}
}

// Log prints the pending action to the logger installed in ctx.
//
// If verbose is true, prints filenames of files that need a repair.
func (am ActionMap) Log(ctx context.Context, verbose bool) {
	am.logSummary(ctx)

	// If have the fancy UI enabled, stop after logging the summary line since the
	// detailed dump is overwhelming with large manifests. We still want it in
	// info-level logs when not using a fancy UI though (since the log is already
	// pretty busy in this case and dumping details doesn't make it much worse.
	// This happens on bots that redirect stderr into a file).
	if ui.GetImplementation(ctx) != nil && !verbose {
		return
	}

	keys := make([]string, 0, len(am))
	for key := range am {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, subdir := range keys {
		actions := am[subdir]

		if subdir == "" {
			logging.Infof(ctx, "In root:")
		} else {
			logging.Infof(ctx, "In subdir %q:", subdir)
		}

		if len(actions.ToInstall) != 0 {
			logging.Infof(ctx, "  to install:")
			for _, pin := range actions.ToInstall {
				logging.Infof(ctx, "    %s", pin)
			}
		}
		if len(actions.ToUpdate) != 0 {
			logging.Infof(ctx, "  to update:")
			for _, pair := range actions.ToUpdate {
				logging.Infof(ctx, "    %s (%s -> %s)",
					pair.From.PackageName, pair.From.InstanceID, pair.To.InstanceID)
			}
		}
		if len(actions.ToRemove) != 0 {
			logging.Infof(ctx, "  to remove:")
			for _, pin := range actions.ToRemove {
				logging.Infof(ctx, "    %s", pin)
			}
		}
		if len(actions.ToRepair) != 0 {
			logging.Infof(ctx, "  to repair:")
			for _, broken := range actions.ToRepair {
				more := broken.RepairPlan.ReinstallReason
				if more == "" {
					more = fmt.Sprintf("%d file(s) to repair", broken.RepairPlan.NumBrokenFiles())
				}
				logging.Infof(ctx, "    %s (%s)", broken.Pin, more)
				if verbose && len(broken.RepairPlan.ToRedeploy) != 0 {
					logging.Infof(ctx, "      to redeploy:")
					for _, f := range broken.RepairPlan.ToRedeploy {
						logging.Infof(ctx, "        %s", f)
					}
				}
				if verbose && len(broken.RepairPlan.ToRelink) != 0 {
					logging.Infof(ctx, "      to relink:")
					for _, f := range broken.RepairPlan.ToRelink {
						logging.Infof(ctx, "        %s", f)
					}
				}
			}
		}
	}
}

func (am ActionMap) logSummary(ctx context.Context) {
	install := 0
	update := 0
	remove := 0
	repair := 0
	for _, actions := range am {
		install += len(actions.ToInstall)
		update += len(actions.ToUpdate)
		remove += len(actions.ToRemove)
		repair += len(actions.ToRepair)
	}

	var work []string
	addWork := func(action string, count int) {
		switch count {
		case 0:
			// Do not log this at all.
		case 1:
			work = append(work, fmt.Sprintf("%s 1 package", action))
		default:
			work = append(work, fmt.Sprintf("%s %d packages", action, count))
		}
	}

	addWork("install", install)
	addWork("update", update)
	addWork("remove", remove)
	addWork("repair", repair)

	switch {
	case len(work) == 0:
		// Noop plan, log nothing. This should not really be happening.
	case len(work) == 1:
		// "Going to install 1 package".
		logging.Infof(ctx, "Going to %s", work[0])
	default:
		// "Going to install 1 package, update 1 package and remove 1 package".
		logging.Infof(ctx, "Going to %s and %s", strings.Join(work[:len(work)-1], ", "), work[len(work)-1])
	}
}

// perPinActions is a "rotated" version of ActionMap where actions are grouped
// by (operation, pin) pairs essentially, not by the subdir they touch.
type perPinActions struct {
	maintenance []pinAction     // don't need package data: removals and relinks
	updates     []updateActions // need package data: installs and repairs
}

// pinAction is an action done to some pin in some subdir.
type pinAction struct {
	action     ActionKind
	pin        common.Pin
	subdir     string
	repairPlan RepairPlan // populated for relinks and repairs only
}

// updateActions is a list of actions that need the package data of this
// specific pin. These are installations (including updates) and repairs.
type updateActions struct {
	pin     common.Pin
	updates []pinAction
}

// perPinActions splits the action map into per-pin action lists.
func (am ActionMap) perPinActions() perPinActions {
	var maintenance []pinAction

	addMaintenanceAction := func(a ActionKind, subdir string, pin common.Pin, plan RepairPlan) {
		maintenance = append(maintenance, pinAction{
			action:     a,
			pin:        pin,
			subdir:     subdir,
			repairPlan: plan,
		})
	}

	var updates []updateActions
	seen := map[common.Pin]int{} // pin => index in `updates`

	addUpdateAction := func(a ActionKind, subdir string, pin common.Pin, plan RepairPlan) {
		idx, ok := seen[pin]
		if !ok {
			updates = append(updates, updateActions{pin: pin})
			idx = len(updates) - 1
			seen[pin] = idx
		}
		updates[idx].updates = append(updates[idx].updates, pinAction{
			action:     a,
			pin:        pin,
			subdir:     subdir,
			repairPlan: plan,
		})
	}

	am.loopOrdered(func(subdir string, actions *Actions) {
		for _, pin := range actions.ToRemove {
			addMaintenanceAction(ActionRemove, subdir, pin, RepairPlan{})
		}
		for _, pin := range actions.ToInstall {
			addUpdateAction(ActionInstall, subdir, pin, RepairPlan{})
		}
		for _, pair := range actions.ToUpdate {
			addUpdateAction(ActionInstall, subdir, pair.To, RepairPlan{})
		}
		for _, broken := range actions.ToRepair {
			if broken.RepairPlan.NeedsReinstall {
				// Need the full reinstall, requires the package data.
				addUpdateAction(ActionInstall, subdir, broken.Pin, broken.RepairPlan)
			} else if len(broken.RepairPlan.ToRedeploy) != 0 {
				// Need to redeploy some files, requires the package data.
				addUpdateAction(ActionRepair, subdir, broken.Pin, broken.RepairPlan)
			} else {
				// Need to symlink existing files, no need to fetch the package data.
				addMaintenanceAction(ActionRelink, subdir, broken.Pin, broken.RepairPlan)
			}
		}
	})

	return perPinActions{maintenance, updates}
}

// repairCB is called for each installed pin to decide whether it should be
// repaired and how.
type repairCB func(subdir string, pin common.Pin) *RepairPlan

// buildActionPlan is used by EnsurePackages to figure out what to install,
// remove, update or repair.
//
// The given 'needsRepair' callback is called for each installed pin to decide
// whether it should be repaired and how.
func buildActionPlan(desired, existing common.PinSliceBySubdir, needsRepair repairCB) (aMap ActionMap) {
	desiredSubdirs := stringset.New(len(desired))
	for desiredSubdir := range desired {
		desiredSubdirs.Add(desiredSubdir)
	}

	existingSubdirs := stringset.New(len(existing))
	for existingSubdir := range existing {
		existingSubdirs.Add(existingSubdir)
	}

	aMap = ActionMap{}

	// all newly added subdirs
	desiredSubdirs.Difference(existingSubdirs).Iter(func(subdir string) bool {
		if want := desired[subdir]; len(want) > 0 {
			aMap[subdir] = &Actions{ToInstall: want}
		}
		return true
	})

	// all removed subdirs
	existingSubdirs.Difference(desiredSubdirs).Iter(func(subdir string) bool {
		if have := existing[subdir]; len(have) > 0 {
			aMap[subdir] = &Actions{ToRemove: have}
		}
		return true
	})

	// all common subdirs
	desiredSubdirs.Intersect(existingSubdirs).Iter(func(subdir string) bool {
		a := Actions{}

		// Figure out what needs to be installed, updated or repaired.
		haveMap := existing[subdir].ToMap()
		for _, want := range desired[subdir] {
			if haveID, exists := haveMap[want.PackageName]; !exists {
				a.ToInstall = append(a.ToInstall, want)
			} else if haveID != want.InstanceID {
				a.ToUpdate = append(a.ToUpdate, UpdatedPin{
					From: common.Pin{PackageName: want.PackageName, InstanceID: haveID},
					To:   want,
				})
			} else if repairPlan := needsRepair(subdir, want); repairPlan != nil {
				a.ToRepair = append(a.ToRepair, BrokenPin{
					Pin:        want,
					RepairPlan: *repairPlan,
				})
			}
		}

		// Figure out what needs to be removed.
		wantMap := desired[subdir].ToMap()
		for _, have := range existing[subdir] {
			if wantMap[have.PackageName] == "" {
				a.ToRemove = append(a.ToRemove, have)
			}
		}

		if !a.Empty() {
			aMap[subdir] = &a
		}
		return true
	})

	if len(aMap) == 0 {
		return nil
	}
	return
}
