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

import { ClusterMetrics } from '@/services/cluster';
import { Metric } from '@/services/metrics';
import {
  MetricThreshold,
  ImpactMetricThreshold,
} from '@/services/project';

export interface PriorityThreshold {
  priority: string;
  thresholds: ImpactMetricThreshold[];
}

export interface Criterium {
  metricName: string;
  metricDescription: string;
  durationKey: string;
  thresholdValue: string;
  actualValue: string;
  satisfied: boolean;
}

export interface PriorityCriteriaResult {
  priority: string;
  satisfied: boolean;
  criteria: Criterium[];
}

export interface PriorityRecommendation {
  recommendation?: PriorityCriteriaResult;
  justification: PriorityCriteriaResult[];
}

function meetsThreshold(actual: string, threshold: string): boolean {
  const actualValue = parseInt(actual, 10);
  const thresholdValue = parseInt(threshold, 10);
  if (isNaN(actualValue) || isNaN(thresholdValue)) {
    return false;
  }
  return actualValue >= thresholdValue;
}

export function createPriorityRecommendation(
    metricValues: ClusterMetrics,
    metrics: Metric[],
    priorities: PriorityThreshold[]): PriorityRecommendation {
  const processedPriorities: PriorityCriteriaResult[] = [];

  // Process the priority thresholds from lowest priority to highest.
  for (let i = priorities.length - 1; i >= 0; i--) {
    const criteria: Criterium[] = [];

    const priorityThresholds = priorities[i].thresholds || [];
    priorityThresholds.forEach((impactMetricThreshold) => {
      const metricId = impactMetricThreshold.metricId;
      const metricValue = metricValues[metricId];
      const metricThreshold: MetricThreshold = impactMetricThreshold.threshold || {};

      // Preferably use the human readable name from the metric definition.
      const metric = metrics.find((m) => m.metricId === metricId);
      const metricName = metric?.humanReadableName || metricId;
      const metricDescription = metric?.description || '(metric description unavailable)';

      if (metricThreshold.oneDay !== undefined) {
        const actual = metricValue?.oneDay?.nominal || '0';
        criteria.push({
          metricName: metricName,
          metricDescription: metricDescription,
          durationKey: '1d',
          thresholdValue: metricThreshold.oneDay,
          actualValue: actual,
          satisfied: meetsThreshold(actual, metricThreshold.oneDay),
        });
      }
      if (metricThreshold.threeDay !== undefined) {
        const actual = metricValue?.threeDay?.nominal || '0';
        criteria.push({
          metricName: metricName,
          metricDescription: metricDescription,
          durationKey: '3d',
          thresholdValue: metricThreshold.threeDay,
          actualValue: actual,
          satisfied: meetsThreshold(actual, metricThreshold.threeDay),
        });
      }
      if (metricThreshold.sevenDay !== undefined) {
        const actual = metricValue?.sevenDay?.nominal || '0';
        criteria.push({
          metricName: metricName,
          metricDescription: metricDescription,
          durationKey: '7d',
          thresholdValue: metricThreshold.sevenDay,
          actualValue: actual,
          satisfied: meetsThreshold(actual, metricThreshold.sevenDay),
        });
      }
    });

    // The priority criteria may be satisfied if at least one criterium has
    // been satisfied.
    let satisfied = criteria.map((c) => c.satisfied).includes(true);

    // Lower priorities' criteria must be satisfied as part of satisfying higher
    // priorities' criteria.
    if (processedPriorities.length > 0) {
      satisfied = satisfied && processedPriorities[processedPriorities.length - 1].satisfied;
    }

    processedPriorities.push({
      priority: priorities[i].priority,
      criteria: criteria,
      satisfied: satisfied,
    });
  }

  // Reverse the priorities so that higher priorities are first.
  processedPriorities.reverse();

  return {
    recommendation: processedPriorities.find((p) => p.satisfied),
    justification: processedPriorities,
  };
}
