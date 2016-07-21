// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package main contains the entry point code for the LUCI Deployment Tool
// ("deploytool"). This tool is a command-line interface designed to perform
// controlled and automated deployment of LUCI (and LUCI-compatible) services.
//
// "deploytool" is Linux-oriented, but may also work on other platforms. It
// leverages external tooling for many remote operations; it is the
// responsibility of the user to have suitable versions of that tooling
// installed and available on PATH.
//
// Overview
//
// The LUCI Deployment Tool loads awareness of services from a
// Layout, which consists of a set of configuration files. These files define:
//	- Source Groups, named collections of Sources, which are versioned
//	  repositories from which deployments are built.
//	- Projects, which are collections of independent deployable Components.
//	- Deployments, which bind a Project to a Source Group and quantify resource
//	  and deployment layouts for those Projects.
//
// All configurations are defined in protobufs, specified at:
// https://github.com/luci/luci-go/tree/master/common/proto/deploy
//
// A layout may define multiple Deployments of the same Project (e.g.,
// "production", "staging", "development"). Each of these may be bound to the
// same or different source groups as per your project's deployment methodology.
//
// When initialized, "deploytool" is pointed to a Layout. It loads all of the
// configuration files in this layout and composes a view of all deployable
// options, asserting the validity of the Layout in the process.
//
// The user specifies the deployments to process, and the tool proceeds to:
//	- Check out the Source Groups referenced by those deployments.
//	  - This includes checking out all of the individual Sources that each
//	    Source Group is composed of.
//	- Loading a Deployment's Project Components from the Source Group and
//	  Sources that the Deployment is bound to.
//
// A fully-initialized Deployment configuration consists of data loaded from the
// initialy Layout combined with data loaded from that Deployment's Sources.
//
// After Deployment configuration is initialized, the deployment process
// consists of the following stages:
//	- Staging: Creating a filesystem space for each staging layout that
//	  hermetically mixes each Component's referenced Sources, derivative
//	  variables, and generated code in a layout conducive to the further
//	  deployment of that component.
//	- Build: Any required local operations are performed on the staging layout
//	  to prepare for deployment.
//	- Push: Remote services are engaged, and the deployed components are
//	  registered and/or installed from the staging area. However, they are not
//	  yet activated.
//	- Commit: The Pushed Components and related configurations are activated
//	  in the remote configuration and the application is deployed.
//
// Layout
//
// The overall set of deployment options, configuration, and associations is
// defined in a central layout configuration. The layout consists of several
// files in sub-directories, each of which define various configuration
// parameters of the larger layout. A layout minimally includes a layout
// configuration file; there is a set of layout-relative paths that are used by
// default, though they can be overridden by the layout file. The default
// hierarchy is:
//	/layout.cfg (Layout configuration file)
//
//	/sources/<source-group> (Defines a source group named "source-group")
//	/sources/<source-group>/<source>.cfg (Defines a source, "source" within
//	                                      "source-group")
//	/sources/<source-group>/... (Additional Sources within "source-group")
//
//	/projects/<project>.cfg (Defines a Project named "project")
//	/projects/... (Additional Projects)
//
//	/deployments/<deployment>.cfg (Defines a Deployment named "deployment")
//	/deployments/... (Additional Deployments)
//
// Each of these files is a text-formatted protobuf, defined in the deployment
// protobuf package:
// https://github.com/luci/luci-go/tree/master/common/proto/deploy/config.proto
//
//	<source>.cfg: "Source" protobuf.
//	<project>.cfg: "Project" protobuf.
//	<deployment>.cfg: "Deployment" protobuf.
//
// Each Project references a set of Components within specific Sources. Those
// Components are, themselves, defined by a text "Component" protobuf.
//
// Source Groups and Sources
//
// A Source Group is a named collection of Source entries. Each Source is,
// itself a named reference to a source repository. Sources are checked out,
// updated, and initialized during "deploytool"'s checkout phase.
//
// At a high level, Projects directly reference Source names (without their
// Source Group), and Deployments directly bind to Source Groups. A Project
// bound to a Deployment, then, uses the Source Group referenced by that
// Deployment and the Source referenced by that Project to arrive at the actual
// source repository that is being referenced.
//
// This allows a single Project to be built from different sources by several
// different Deployments (e.g., development, staging, and production). This is
// accomplished by having each of those Deployments reference a different Source
// Group, but having each of those Source Groups contain a Source definition
// for each Source referenced by the project. For example:
//	/sources/production/myrepo.cfg
//	/sources/staging/myrepo.cfg
//	/sources/development/myrepo.cfg
//	/projects/myproject.cfg
//	/deployments/myproject-production.cfg
//	/deployments/myproject-staging.cfg
//	/deployments/myproject-development.cfg
//
// The "myproject.cfg" would reference a Source named "myrepo". Any given
// Deployment's projection of the Project would select a different Source, with
// "myproject-production" referencing "production/myrepo", "myproject-staging"
// referencing "staging/myrepo", and "myproject-development" referencing
// "development/myrepo".
//
// Source repositories may include a source initialization file in their root,
// called ".luci-deploytool.cfg". This file contains a text "SourceLayout"
// protobuf, and allows a source to self-describe and perform post-checkout
// initialization.
//
//	NOTE: For security reasons, initialization scripts will not be permitted to
//	run unless the Source entry describing that repository has its
//	"run_init_scripts" value set to "true".
//
// Staging
//
// A staging space is created for each deployable Component. Staging offers
// "deploytool" an isolated canvas with which it can the filesystem layouts for
// that Component's actual deployment tooling to operate. All file operations,
// generation, and structuring are performed in the staging state such that the
// resulting staging directory is available for tooling or humans to use.
//
// The staging space for a given Component depends on that Component's type.
// A staging layout is intended to be human-navigatable while conforming to any
// layout requirements imposed by that Component's deployment tooling.
//
// One goal that is enforced is that the actual checkout directories used by
// any given Component are considered read-only. This means that staging for
// Components which require generated files to exist alongside source must
// copy or mirror that source elsewhere. Some of the more convoluted aspects of
// staging layouts are the result of this requirement.
//
// Staging - AppEngine - Generic
//
// AppEngine deployments are composed of two sets of information:
//	- Individual AppEngine modules, including the default module.
//	- AppEngine Project-wide globals such as Index, Cron, Dispatch, and Queue
//	  settings.
//
// The individual Components are staged and deployed independently. The globals
// are composed of their respective settings in each individual Component
// associated with the AppEngine project regardless of whether that Component is
// actually being deployed. The aggregate globals are re-asserted once per
// cloud project at the end of module deployment.
//
// References to static content are flattened into static directories within the
// staging area. The generated YAML files are configured to point to this
// flattened space regardless of the original static content's location within
// the source.
//
// Staging - AppEngine - dev_appserver
//
// Moving Component configuration into protobufs and constructing composite
// GOPATH and generated configuration files prevents AppEngine tooling from
// working out of the box in the source repository.
//
// The offered solution is to manually stage the Components under test, then
// run tooling against the staged Component directories. The user may optionally
// install the staged paths (GOPATH, etc.) or use their default environment's
// paths.
//
// One downside to this solution is that staging operates on a snapshot of the
// repository, meaning that changes to the repository won't be reflected in the
// staged environment. The user can address this by either manually re-staging
// the Deployment when a file changes. The user may also structure their
// application such that the staged content doesn't change frequently (e.g.,
// for Go, have a simple entry point that immediately imports the main app
// logic). Specific structures depend on the type of Component and how it is
// staged (see below).
//
// In the future, a command to bootstrap "dev_appserver" through the staged
// environment would be useful.
//
// Staging - AppEngine Classic - Go
//
// Go Classic AppEngine Components are deployed using the `appcfg.py` tool,
// which is the fastest available method to deploy such applications.
//
// Because the "app.yaml" file must exist alongside the deployed source, a
// stub entry point is generated in the staging area. This stub simply imports
// the actual entry point package.
//
// The Component's Sources which declare GOPATH presence are combined in a
// virutal GOPATH within the Component's staging area. This GOPATH is then
// installed and `appcfg.py`'s "update" method is invoked to upload the
// Component.
//
// Staging - AppEngine Managed VM - Go
//
// Go AppEngine Managed VMs use a set of tools for deployment:
//	- `aedeploy`, an AppEngine project tool which copies the various referenced
//	  sources across GOPATH entries into a single GOPATH hierarchy for Docker
//	  isolation.
//	- `gcloud`, which engages the remote AppEngine service and offers deployment
//	  utility.
//	- Behind the scenes, `gcloud` uses `docker` to build the actual deployed
//	  image from the source (`aedeploy` target) and assembled GOPATH.
//
// Because the "app.yaml" file must exist alongside the entry point code, the
// contents of the entry package are copied (via symlink) into a generated
// entry point package in the staging area.
//
// The Component's Sources which declare GOPATH presence are combined in a
// virutal GOPATH within the Component's staging area. This GOPATH is then
// collapsed by `aedeploy` when the Managed VM image is built.
//
//	NOTE: because the entry point is actually a clone of the entry point
//	package, "internal/" imports will not work. This can be easily worked around
//	by having the entry point import another non-internal package within the
//	project, and having that package act as the actual entry point.
//
// Staging - AppEngine - Static Content Module
//
// The "deploytool" supports the concept of a static content module. This is an
// AppEngine module whose sole purpose is to, via AppEngine handler definitions,
// map to uploaded static content. The utility of a static module is that it can
// be effortlessly updated without impacting actual AppEngine runtime processes.
// Static modules can be used in conjunction with module-referencing handlers in
// the default AppEngine module to create the effect of the default module
// actually hosting the static content.
//
// Static content modules are staged like other AppEngine modules, only with
// no running code.
//
// Staging - Container Engine - Generic
//
// The "deploytool" supports depoying services to Google Container Engine, which
// is backed by Kubernetes. A project's Container Engine configuration consists
// of a series of Container Engine clusters, each of which hosts a series of
// homogenous machines. Kubernetes Pods, each of which are composed of one or
// more Kubernetes Components (i.e., Docker images), are deployed to one or more
// Google Container Engine Clusters.
//
// The Deployment Component defines a series of Container Engine Pods, an
// amalgam of a Kubernetes Pod definition and Container Engine requirements of
// that Kubernetes Pod. Each Kubernetes Pod's Component is built in its own
// staging area and deployed as part of that Pod to one or more Container Engine
// clusters.
//
// The Build phase of Container Engine deployment constructs a local Docker
// image of each Kubernetes Component. The Push phase pushes those images to
// the remote Docker image service. The Commit phase enacts the generated
// Kubernetes/ configuration which references those images on the Container
// Engine configuration.
//
// Container Engine management is done in two layers: firstly, the Container
// Engine configuration is managed with `gcloud` commands. This configures:
//	- Which managed Clusters exist on Google Container Engine.
//	- What scopes, system specs, and node count each Cluster has.
//
// Within a Cluster, several managed Kubernetes pods are deployed. The
// deployment is done using the `kubectl` tool, selecting the cluster using
// the "--context" flag to select the `gcloud`-generated Kubernetes context.
//
// Each "deploytool" Component (Kubernetes Pod, at this level) is managed as a
// Kubernetes Deployment (http://kubernetes.io/docs/user-guide/deployments/). A
// "deploytool"-managed Deployment will have the following metadata annotations:
//	- "luci.managedBy", set to "luci-deploytool"
//	- "luci.deploytool/version" set to the "deploytool" Deployment's version
//	  string.
//	- "luci.deploytool/sourceVersion" set to the revision of the Deployment
//	  Component's source.
//
// The Kubernetes Deployment for a Component will be named
// "<project>--<component>".
//
// "deploytool"-driven Kubernetes depoyment is fairly straightforward:
//	- Use "kubectl get deployments/<name>" to get the current Deployment state.
//	- If there is a current Deployment,
//	  - If its "luci.managedBy" annotation doesn't equal "luci-deploytool",
//	    fail. The user must manually correct this situation.
//	  - Check if the "luci.deploytool/version" matches the container version.
//	    If it does, succeed.
//	- Create/update the Deployment's configuration using "kubectl apply".
//
// Staging - Container Engine Pod Components - Go
//
// Go Container Engine Pods use a set of tools for deployment:
//	- `aedeploy`, an AppEngine project tool which copies the various referenced
//	  sources across GOPATH entries into a single GOPATH hierarchy for Docker
//	  isolation.
//	- `gcloud`, which engages the remote AppEngine service and offers deployment
//	  utility.
//	- `docker`, which is used to build and manage the Docker images.
//
// The Component's Sources which declare GOPATH presence are combined in a
// virutal GOPATH within the Component's staging area. This GOPATH is then
// collapsed by `aedeploy` when the Docker image is built.
//
// The actual Component is built directly from the Source using `aedeploy` and
// `docker build`. This is acceptable, since this is a read-only operation and
// will not modify the Source.
//
// To Do
//
// Following are some ideas of "planned" features that would be useful to add
// to "deploytool":
//	- Add a "manage" command to manage a specific Deployment and/or Component.
//	  Each invocation would load special set of subcommands based on that
//	  resource:
//	  - If the resource is a Deployment, query status?
//	  - If the resource is a Kubernetes Component, offer:
//	    - "kubectl" fall-through, automatically specifying the right Kubernetes
//	      Context for the Component's Compute Engine Cluster.
//	    - Other common automatable management macros.
//	  - Offer a "dev_appserver" fallthrough to create a staging area and
//	    bootstrap"dev_appserver" for the named staged AppEngine Components.
package main
