// Copyright 2016 The LUCI Authors.
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

package main

import (
	"fmt"
	"strconv"
	"strings"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/deploytool/api/deploy"

	"github.com/maruel/subcommands"
	"golang.org/x/net/context"
)

var cmdManage = subcommands.Command{
	UsageLine: "manage [DEPLOYMENT.[.COMPONENT]]... -help|SUBCOMMANDS...",
	ShortDesc: "Management subcommands for deployed components.",
	LongDesc:  "Offers subcommands to manage a Deployment or Component.",
	CommandRun: func() subcommands.CommandRun {
		return &cmdManageRun{}
	},
}

type cmdManageRun struct {
	subcommands.CommandRunBase
}

func (cmd *cmdManageRun) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	a, c := app.(*application), cli.GetContext(app, cmd, env)

	if len(args) == 0 {
		log.Errorf(c, "Must supply a Deployment or Component to operate on.")
		return 1
	}

	// Load our frozen checkout.
	if err := a.layout.loadFrozenLayout(c); err != nil {
		logError(c, err, "Failed to load frozen checkout")
		return 1
	}

	// Resolve to the specified Deployment or Component.
	mApp, err := getManager(c, a, args[0])
	if err != nil {
		logError(c, err, "Failed to resolve management target.")
		return 1
	}
	if mApp == nil {
		fmt.Printf("no management options available for %q", args[0])
		return 0
	}
	return subcommands.Run(mApp, args[1:])
}

type manageApp struct {
	*application

	subcommands []*subcommands.Command
	dp          deploymentPlan
}

func (a *manageApp) GetName() string                     { return a.application.GetName() + " manage" }
func (a *manageApp) GetCommands() []*subcommands.Command { return a.subcommands }

func getManager(c context.Context, a *application, name string) (*manageApp, error) {
	mApp := manageApp{
		application: a,
		subcommands: []*subcommands.Command{
			subcommands.CmdHelp,
		},
	}

	switch dep, comp, err := a.layout.getDeploymentComponent(name); {
	case err != nil:
		return nil, err

	case comp != nil:
		mApp.dp.addProjectComponent(comp)
		err := a.runWork(c, func(w *work) error {
			return mApp.dp.initialize(w, &a.layout)
		})
		if err != nil {
			return nil, errors.Annotate(err, "failed to initialize deployment plan").Err()
		}

		switch comp.GetComponent().(type) {
		case *deploy.Component_AppengineModule:
			break
		case *deploy.Component_GkePod:
			mApp.subcommands = append(mApp.subcommands, []*subcommands.Command{
				{
					UsageLine: "kubectl <args...>",
					ShortDesc: "run a kubectl command",
					LongDesc:  "Run a kubectl command in this component's Container Engine context.",
					CommandRun: func() subcommands.CommandRun {
						cr := manageGKEPodKubectlCommandRun{
							comp: comp,
						}
						cr.GetFlags().StringVar(&cr.cluster, "cluster", cr.cluster,
							"The cluster to operate on. If only one cluster is bound, this can be left blank.")
						return &cr
					},
				},

				{
					UsageLine: "create-cluster",
					ShortDesc: "create the configured Google Container Engine cluster",
					LongDesc:  "Run a gcloud command to create the configured Google Container Engine cluster.",
					CommandRun: func() subcommands.CommandRun {
						cr := manageGKEPodCreateClusterCommandRun{
							comp: comp,
						}

						flags := cr.GetFlags()
						flags.StringVar(&cr.cluster, "cluster", cr.cluster,
							"The cluster to create. If only one cluster is bound, this can be left blank.")
						flags.BoolVar(&cr.dryRun, "dry-run", cr.dryRun,
							"Generate and print the commands, but don't actually run them.")
						return &cr
					},
				},
			}...)
		default:
			break
		}

	default:
		mApp.dp.addDeployment(dep)
		err := a.runWork(c, func(w *work) error {
			return mApp.dp.initialize(w, &a.layout)
		})
		if err != nil {
			return nil, errors.Annotate(err, "failed to initialize deployment plan").Err()
		}

		if cp := dep.cloudProject; cp != nil {
			mApp.subcommands = append(mApp.subcommands, &subcommands.Command{
				UsageLine: "update_appengine",
				ShortDesc: "update AppEngine parameters",
				LongDesc:  "Update AppEngine cron, dispatch, index, and queue configurations.",
				CommandRun: func() subcommands.CommandRun {
					return &updateGAECommandRun{
						project: cp,
					}
				},
			})
		}
		break
	}

	return &mApp, nil
}

////////////////////////////////////////////////////////////////////////////////
// Manage / GKE Pod
////////////////////////////////////////////////////////////////////////////////

type manageGKEPodKubectlCommandRun struct {
	subcommands.CommandRunBase

	comp    *layoutDeploymentComponent
	cluster string
}

func (cmd *manageGKEPodKubectlCommandRun) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	a, c := app.(*manageApp), cli.GetContext(app, cmd, env)

	// Figure out which cluster to use.
	bp, err := getPodBindingForCluster(cmd.comp, cmd.cluster)
	if err != nil {
		logError(c, err, "Failed to identify cluster.")
		return 1
	}

	var rv int
	err = a.runWork(c, func(w *work) error {
		kubeCtx, err := getContainerEngineKubernetesContext(w, bp.cluster)
		if err != nil {
			return errors.Annotate(err, "").Err()
		}

		kubectl, err := w.tools.kubectl(kubeCtx)
		if err != nil {
			return errors.Annotate(err, "").Err()
		}

		rv, err = kubectl.exec(args...).forwardOutput().run(w)
		return err
	})
	if err != nil {
		logError(c, err, "Failed to run kubectl command.")
		if rv == 0 {
			rv = 1
		}
	}
	return rv
}

type manageGKEPodCreateClusterCommandRun struct {
	subcommands.CommandRunBase

	comp    *layoutDeploymentComponent
	cluster string
	dryRun  bool
}

func (cmd *manageGKEPodCreateClusterCommandRun) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	a, c := app.(*manageApp), cli.GetContext(app, cmd, env)

	// Figure out which cluster to use.
	bp, err := getPodBindingForCluster(cmd.comp, cmd.cluster)
	if err != nil {
		logError(c, err, "Failed to identify cluster.")
		return 1
	}

	// Build our "gcloud" args.
	//
	// We use "beta" because "--enable-autoupgrade" is currently a beta feature.
	gcloudArgs := []string{
		"beta", "container", "clusters", "create",
		"--zone", bp.cluster.Zone,
		"--num-nodes", strconv.FormatInt(int64(bp.cluster.Nodes), 10),
	}
	if v := bp.pod.Scopes; len(v) > 0 {
		gcloudArgs = append(gcloudArgs, []string{"--scopes", strings.Join(v, ",")}...)
	}
	if v := bp.cluster.MachineType; v != "" {
		gcloudArgs = append(gcloudArgs, []string{"--machine-type", v}...)
	}
	if v := bp.cluster.DiskSizeGb; v > 0 {
		gcloudArgs = append(gcloudArgs, []string{"--disk-size", strconv.FormatInt(int64(v), 10)}...)
	}
	if bp.cluster.EnableAutoUpgrade {
		gcloudArgs = append(gcloudArgs, "--enable-autoupgrade")
	}
	gcloudArgs = append(gcloudArgs, bp.cluster.Name)

	if cmd.dryRun {
		log.Infof(c, "(Dry run) Generated 'gcloud' command: gcloud %s", strings.Join(gcloudArgs, " "))
		return 0
	}

	var rv int
	err = a.runWork(c, func(w *work) (err error) {
		cloudProjectName := bp.pod.comp.dep.cloudProject.Name
		gcloud, err := w.tools.gcloud(cloudProjectName)
		if err != nil {
			return errors.Annotate(err, "").Err()
		}

		if rv, err = gcloud.exec(gcloudArgs[0], gcloudArgs[1:]...).run(w); err != nil {
			return errors.Annotate(err, "failed to run 'gcloud' command").Err()
		}
		return nil
	})
	if err != nil {
		logError(c, err, "Failed to run gcloud command.")
		if rv == 0 {
			rv = 1
		}
	}
	return rv
}

func getPodBindingForCluster(comp *layoutDeploymentComponent, cluster string) (*layoutDeploymentGKEPodBinding, error) {
	var bp *layoutDeploymentGKEPodBinding
	switch {
	case cluster != "":
		cluster := comp.dep.cloudProject.gkeClusters[cluster]
		if cluster == nil {
			return nil, errors.Reason("invalid GKE cluster name: %q", cluster).Err()
		}
		for _, gkeBP := range comp.gkePods {
			if gkeBP.cluster == cluster {
				bp = gkeBP
				break
			}
		}

	case len(comp.gkePods) == 0:
		return nil, errors.New("pod is not bound to any clusters")

	case len(comp.gkePods) == 1:
		bp = comp.gkePods[0]

	default:
		clusters := make([]string, len(comp.gkePods))
		for i, cluster := range comp.gkePods {
			clusters[i] = fmt.Sprintf("- %s", cluster.cluster.Name)
		}

		return nil, errors.Reason("the Kubernetes pod is bound to multiple clusters. Specify one with -cluster").
			InternalReason("clusters(%v)", clusters).Err()
	}
	if bp == nil {
		return nil, errors.New("Could not identify cluster for pod.")
	}
	return bp, nil
}

////////////////////////////////////////////////////////////////////////////////
// Manage / GAE
////////////////////////////////////////////////////////////////////////////////

type updateGAECommandRun struct {
	subcommands.CommandRunBase

	project *layoutDeploymentCloudProject
}

func (cmd *updateGAECommandRun) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	a, c := app.(*manageApp), cli.GetContext(app, cmd, env)

	// Do not deploy any actual GAE modules.
	//
	// However, without modules, the "commit" stage will refrain from pushing new
	// config, so force that.
	a.dp.reg.appEngineModulesOnly()
	for _, gae := range a.dp.reg.gaeProjects {
		gae.alwaysCommitGAEConfig = true
		gae.clearModules()
	}

	var rv int
	err := a.runWork(c, func(w *work) error {
		a.dp.params.stage = deployCommit
		return a.dp.deploy(w)
	})
	if err != nil {
		logError(c, err, "Failed to update GAE parameters.")
		return 1
	}
	return rv
}
