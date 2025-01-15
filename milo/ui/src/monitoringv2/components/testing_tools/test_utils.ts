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

import {
  BuilderAlert,
  buildStructuredAlerts,
  GenericAlert,
  GroupedAlertKeys,
  OneBuildHistory,
  OneTestHistory,
  StepAlert,
  StructuredAlert,
  TestAlert,
} from '@/monitoringv2/util/alerts';
import { builderPath } from '@/monitoringv2/util/server_json';
import { BuilderID } from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_common.pb';
import { Status } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';
import { TestVariantStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';

const defaultBuilderID = {
  project: 'chromium',
  bucket: 'ci',
  builder: 'linux-rel',
};

export class BuilderAlertBuilder {
  private builderAlert: BuilderAlert = {
    kind: 'builder',
    key: '',
    builderID: defaultBuilderID,
    consecutiveFailures: 2,
    consecutivePasses: 0,
    history: [],
  };

  withBuilderID(builderID: BuilderID): this {
    this.builderAlert.builderID = builderID;
    return this;
  }

  withBuilder(builder: string): this {
    this.builderAlert.builderID = { ...this.builderAlert.builderID, builder };
    return this;
  }

  withConsecutiveFailures(consecutiveFailures: number): this {
    this.builderAlert.consecutiveFailures = consecutiveFailures;
    return this;
  }

  withConsecutivePasses(consecutivePasses: number): this {
    this.builderAlert.consecutivePasses = consecutivePasses;
    return this;
  }

  pushHistory(history: OneBuildHistory): this {
    this.builderAlert.history.push(history);
    return this;
  }

  build(): BuilderAlert {
    if (
      !this.builderAlert.builderID.project ||
      !this.builderAlert.builderID.bucket ||
      !this.builderAlert.builderID.builder
    ) {
      throw new Error('Project, bucket, and builder must be specified.');
    }
    this.builderAlert.key = builderPath(this.builderAlert.builderID);
    return this.builderAlert;
  }
}

export class OneBuildHistoryBuilder {
  private oneBuildHistory: OneBuildHistory = {
    buildId: '123',
    status: Status.SUCCESS,
    startTime: new Date().toISOString(),
    summaryMarkdown: 'successful build',
  };

  withBuildId(buildId: string): this {
    this.oneBuildHistory.buildId = buildId;
    return this;
  }

  withStatus(status: Status | undefined): this {
    this.oneBuildHistory.status = status;
    return this;
  }

  withStartTime(startTime: string | undefined): this {
    this.oneBuildHistory.startTime = startTime;
    return this;
  }

  withSummaryMarkdown(summaryMarkdown: string | undefined): this {
    this.oneBuildHistory.summaryMarkdown = summaryMarkdown;
    return this;
  }

  build(): OneBuildHistory {
    if (!this.oneBuildHistory.buildId) {
      throw new Error('buildId must be specified.');
    }
    return this.oneBuildHistory;
  }
}

export class StepAlertBuilder {
  private stepAlert: StepAlert = {
    kind: 'step',
    key: '',
    builderID: defaultBuilderID,
    stepName: 'unit_tests',
    consecutiveFailures: 2,
    consecutivePasses: 0,
    history: [],
  };

  withBuilderID(builderID: BuilderID): this {
    this.stepAlert.builderID = builderID;
    return this;
  }

  withBuilder(builder: string): this {
    this.stepAlert.builderID = { ...this.stepAlert.builderID, builder };
    return this;
  }

  withStepName(stepName: string): this {
    this.stepAlert.stepName = stepName;
    return this;
  }

  withConsecutiveFailures(consecutiveFailures: number): this {
    this.stepAlert.consecutiveFailures = consecutiveFailures;
    return this;
  }

  withConsecutivePasses(consecutivePasses: number): this {
    this.stepAlert.consecutivePasses = consecutivePasses;
    return this;
  }

  pushHistory(history: OneBuildHistory): this {
    this.stepAlert.history.push(history);
    return this;
  }

  build(): StepAlert {
    if (
      !this.stepAlert.builderID.project ||
      !this.stepAlert.builderID.bucket ||
      !this.stepAlert.builderID.builder ||
      !this.stepAlert.stepName
    ) {
      throw new Error(
        'Project, bucket, builder, and stepName must be specified.',
      );
    }
    this.stepAlert.key = `${builderPath(this.stepAlert.builderID)}/${this.stepAlert.stepName}`;
    return this.stepAlert;
  }
}

export class TestAlertBuilder {
  private testAlert: TestAlert = {
    kind: 'test',
    key: '',
    builderID: defaultBuilderID,
    stepName: 'unit_tests',
    testName: 'hello_world_test',
    testId: 'ninja://hello_world_test',
    variantHash: '1234',
    consecutiveFailures: 2,
    consecutivePasses: 0,
    history: [],
  };

  withBuilderID(builderID: BuilderID): this {
    this.testAlert.builderID = builderID;
    return this;
  }

  withBuilder(builder: string): this {
    this.testAlert.builderID = { ...this.testAlert.builderID, builder };
    return this;
  }

  withStepName(stepName: string): this {
    this.testAlert.stepName = stepName;
    return this;
  }

  withTestName(testName: string): this {
    this.testAlert.testName = testName;
    return this;
  }

  withTestId(testId: string): this {
    this.testAlert.testId = testId;
    return this;
  }

  withVariantHash(variantHash: string): this {
    this.testAlert.variantHash = variantHash;
    return this;
  }

  withConsecutiveFailures(consecutiveFailures: number): this {
    this.testAlert.consecutiveFailures = consecutiveFailures;
    return this;
  }

  withConsecutivePasses(consecutivePasses: number): this {
    this.testAlert.consecutivePasses = consecutivePasses;
    return this;
  }

  pushHistory(history: OneTestHistory): this {
    this.testAlert.history.push(history);
    return this;
  }

  build(): TestAlert {
    if (
      !this.testAlert.builderID.project ||
      !this.testAlert.builderID.bucket ||
      !this.testAlert.builderID.builder ||
      !this.testAlert.stepName ||
      !this.testAlert.testName ||
      !this.testAlert.testId ||
      !this.testAlert.variantHash
    ) {
      throw new Error(
        'Project, bucket, builder, stepName, testName, testId, and variantHash must be specified.',
      );
    }

    // Construct a key in this specific format
    // eslint-disable-next-line max-len
    this.testAlert.key = `${builderPath(this.testAlert.builderID)}/${this.testAlert.stepName}/${this.testAlert.testName}/${this.testAlert.variantHash}`;

    return this.testAlert;
  }
}

export class OneTestHistoryBuilder {
  private oneTestHistory: OneTestHistory = {
    buildId: '123',
    status: TestVariantStatus.UNEXPECTED,
    startTime: new Date().toISOString(),
    failureReason: 'failure reason',
  };

  withBuildId(buildId: string): this {
    this.oneTestHistory.buildId = buildId;
    return this;
  }

  withStatus(status: TestVariantStatus | undefined): this {
    this.oneTestHistory.status = status;
    return this;
  }

  withStartTime(startTime: string | undefined): this {
    this.oneTestHistory.startTime = startTime;
    return this;
  }

  withFailureReason(failureReason: string | undefined): this {
    this.oneTestHistory.failureReason = failureReason;
    return this;
  }

  build(): OneTestHistory {
    if (!this.oneTestHistory.buildId) {
      // buildId is now required
      throw new Error('buildId must be specified.');
    }
    return this.oneTestHistory;
  }
}

export const groupAlerts = (
  alertGroups: GenericAlert[][],
): GroupedAlertKeys => {
  return Object.fromEntries(
    alertGroups.map((alerts, i) => [i.toString(), alerts.map((a) => a.key)]),
  );
};

// Legacy exports, prefer to use the builders above + buildStructuredAlerts
export const testBuilderAlert: BuilderAlert = {
  kind: 'builder',
  key: builderPath(defaultBuilderID),
  builderID: defaultBuilderID,
  consecutiveFailures: 2,
  consecutivePasses: 0,
  history: [],
};

export const testStepAlert: StepAlert = {
  kind: 'step',
  key: `${builderPath(defaultBuilderID)}/compile`,
  builderID: defaultBuilderID,
  stepName: 'compile',
  consecutiveFailures: 2,
  consecutivePasses: 0,
  history: [],
};

export const testAlerts: StructuredAlert[] = buildStructuredAlerts([
  testBuilderAlert,
  testStepAlert,
]);

const builderID2 = {
  project: 'chromium',
  bucket: 'ci',
  builder: 'win-rel',
};

export const testBuilderAlert2: BuilderAlert = {
  kind: 'builder',
  key: builderPath(builderID2),
  builderID: builderID2,
  consecutiveFailures: 2,
  consecutivePasses: 0,
  history: [],
};

export const testAlerts2: StructuredAlert[] = buildStructuredAlerts([
  testBuilderAlert2,
]);
