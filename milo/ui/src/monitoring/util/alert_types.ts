// Copyright 2024 The LUCI Authors.
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

export const AlertTypes = {
  // HungBuilder indicates that a builder has been executing a step for too
  // long.
  HungBuilder: 'hung-builder',

  // OfflineBuilder indicates that we have no recent updates from the builder.
  OfflineBuilder: 'offline-builder',

  // IdleBuilder indicates that a builder has not executed any builds recently
  // even though it has requests queued up.
  IdleBuilder: 'idle-builder',

  // InfraFailure indicates that a builder step failed due to infrastructure
  // problems rather than errors in the code it tried to build or execute.
  InfraFailure: 'infra-failure',

  // BuildFailure indicates that one of the build steps failed, must likely
  // due to the patch it's building/running with.
  BuildFailure: 'build-failure',
};

export const AlertSeverity = {
  TreeCloser: 0,
  // HungBuilder is an alert about a builder being hung (stuck running a
  // particular step)
  HungBuilder: 2,
  // InfraFailure is an infrastructure failure. It is higher severity than a
  // reliable failure because if there is an infrastructure failure, the test
  // code is not even run, and so we are losing data about if the tests pass or
  // not.
  InfraFailure: 3,
  // ReliableFailure is a failure which has shown up multiple times.
  ReliableFailure: 4,
  // NewFailure is a failure which just started happening.
  NewFailure: 5,
  // IdleBuilder is a builder which is "idle" (buildbot term) and which has
  // above a certain threshold of pending builds.
  IdleBuilder: 6,
  // OfflineBuilder is a builder which is offline.
  OfflineBuilder: 7,
  // NoSeverity is a placeholder Severity value which means nothing. Used by
  // analysis to indicate that it doesn't have a particular Severity to assign
  // to an alert.
  NoSeverity: 8,

  // Resolved indicates alerts that have been previously resolved.
  Resolved: 10000,
};

export const SeverityTooltips = [
  // TreeCloser
  `Tree closers are high priority issues that closed the tree.`,
  // HungBuilder
  `Hung builder alerts tell you about builders which are stuck running a specific step. These alerts should be handled by troopers.`,
  // InfraFailure
  `Infra failures are problems with the underlying infrastructure that runs tests and builds. These alerts should be handled by troopers.`,
  // ReliableFailure
  `Reliable failures are failures which have shown up several times.`,
  // NewFailure
  `New failures are failures which just recently started happening. Sometimes, these failures can be flaky.`,
  // IdleBuilder
  `Idle builders are builders that have a number pending builds exceeding a set threshold. These failures should be handled by a trooper.`,
  // OfflineBuilder
  `Offline builders are builders which are offline. These failures should be handled by a trooper.`,
  // NoSeverity
  `This category describes alerts which don't have a severity assigned to them.`,
];

const TrooperAlertTypes = [
  AlertTypes.InfraFailure,
  AlertTypes.OfflineBuilder,
  AlertTypes.HungBuilder,
  AlertTypes.IdleBuilder,
];

export const getSeverityTooltip = function (alertType: number) {
  if (alertType in SeverityTooltips) {
    return SeverityTooltips[alertType];
  }
  return '';
};

export const isTrooperAlertType = function (alertType: string) {
  return TrooperAlertTypes.indexOf(alertType) != -1;
};
