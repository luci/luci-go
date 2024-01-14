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

import { Cluster_TimewiseCounts } from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';
import { ProjectMetric } from '@/proto/go.chromium.org/luci/analysis/proto/v1/metrics.pb';
import { BugManagementPolicy } from '@/proto/go.chromium.org/luci/analysis/proto/v1/projects.pb';

export interface Criterium {
  metricId: string;
  metricName: string;
  metricDescription: string;
  durationKey: string; // '1d', '3d' or '7d' (for 1 day, 3 day or 7 days).
  greaterThanOrEqual: boolean; // Whether this is a metric_value >= threshold criteria. If not, it is a metric_value < threshold.
  thresholdValue: string;
  currentValue: string;
  satisfied: boolean;
}

export const criteriaForPolicy = (policy: BugManagementPolicy, metricDefinitions: ProjectMetric[], metricValues: {[key: string]: Cluster_TimewiseCounts} | undefined, activationCriteria: boolean) : Criterium[] => {
  const result : Criterium[] = [];
  policy.metrics.forEach((m) => {
    const metricDefinition = metricDefinitions.find((d) => d.metricId == m.metricId);
    const threshold = activationCriteria ? m.activationThreshold : m.deactivationThreshold;
    const currentTimewiseCounts : Cluster_TimewiseCounts | undefined = metricValues ? metricValues[m.metricId] : undefined;

    const metricName = metricDefinition?.humanReadableName || m.metricId;
    const metricDescription = metricDefinition?.description || '(metric description unavailable)';

    if (threshold?.oneDay !== undefined) {
      let currentValue : string|undefined;
      if (currentTimewiseCounts) {
        currentValue = currentTimewiseCounts.oneDay?.nominal || '0';
      }
      result.push({
        metricId: m.metricId,
        metricName: metricName,
        metricDescription: metricDescription,
        durationKey: '1d',
        greaterThanOrEqual: activationCriteria,
        thresholdValue: threshold.oneDay,
        currentValue: currentValue || '-',
        satisfied: isCriteriumSatisfied(currentValue, threshold.oneDay, activationCriteria),
      });
    }
    if (threshold?.threeDay !== undefined) {
      let currentValue : string|undefined;
      if (currentTimewiseCounts) {
        currentValue = currentTimewiseCounts.threeDay?.nominal || '0';
      }
      result.push({
        metricId: m.metricId,
        metricName: metricName,
        metricDescription: metricDescription,
        durationKey: '3d',
        greaterThanOrEqual: activationCriteria,
        thresholdValue: threshold.threeDay,
        currentValue: currentValue || '-',
        satisfied: isCriteriumSatisfied(currentValue, threshold.threeDay, activationCriteria),
      });
    }
    if (threshold?.sevenDay !== undefined) {
      let currentValue : string|undefined;
      if (currentTimewiseCounts) {
        currentValue = currentTimewiseCounts.sevenDay?.nominal || '0';
      }
      result.push({
        metricId: m.metricId,
        metricName: metricName,
        metricDescription: metricDescription,
        durationKey: '7d',
        greaterThanOrEqual: activationCriteria,
        thresholdValue: threshold.sevenDay,
        currentValue: currentValue || '-',
        satisfied: isCriteriumSatisfied(currentValue, threshold.sevenDay, activationCriteria),
      });
    }
  });
  return result;
};

const isCriteriumSatisfied = (current: string|undefined, threshold: string, greaterThanOrEqual: boolean): boolean => {
  const actualValue = parseInt(current || '', 10);
  const thresholdValue = parseInt(threshold, 10);
  if (isNaN(actualValue) || isNaN(thresholdValue)) {
    return false;
  }
  if (greaterThanOrEqual) {
    return actualValue >= thresholdValue;
  } else {
    return actualValue < thresholdValue;
  }
};
