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
import { getMockMetricsList } from '@/testing_tools/mocks/metrics_mock';

import {
  createPriorityRecommendation,
  PriorityThreshold,
} from './priority_recommendation';

describe('Test createPriorityRecommendation', () => {
  const metricValues: ClusterMetrics = {
    'human-cls-failed-presubmit': {
      oneDay: { nominal: '98' },
      threeDay: { nominal: '158' },
      sevenDay: { nominal: '167' },
    },
    'critical-failures-exonerated': {
      oneDay: { nominal: '0' },
      threeDay: { nominal: '0' },
      sevenDay: { nominal: '0' },
    },
    'failures': {
      oneDay: { nominal: '7625' },
      threeDay: { nominal: '16052' },
      sevenDay: { nominal: '15800' },
    },
  };
  const metrics: Metric[] = getMockMetricsList('testproject');
  const priorities: PriorityThreshold[] = [
    {
      priority: 'P0',
      thresholds: [
        {
          metricId: 'human-cls-failed-presubmit',
          threshold: {
            oneDay: '20',
          },
        },
      ],
    },
    {
      priority: 'P1',
      thresholds: [
        {
          metricId: 'human-cls-failed-presubmit',
          threshold: {
            oneDay: '10',
          },
        },
      ],
    },
    {
      priority: 'P2',
      thresholds: [
        {
          metricId: 'human-cls-failed-presubmit',
          threshold: {
            sevenDay: '1',
          },
        },
        {
          metricId: 'critical-failures-exonerated',
          threshold: {
            sevenDay: '1',
          },
        },
      ],
    },
  ];

  it('handles empty metric values', () => {
    const r = createPriorityRecommendation({}, metrics, priorities);
    expect(r.recommendation).toBe(undefined);
    expect(r.justification).toEqual([
      {
        priority: 'P0',
        satisfied: false,
        criteria: [
          {
            metricName: 'User Cls Failed Presubmit',
            metricDescription: 'User Cls Failed Presubmit Description',
            durationKey: '1d',
            thresholdValue: '20',
            actualValue: '0',
            satisfied: false,
          },
        ],
      },
      {
        priority: 'P1',
        satisfied: false,
        criteria: [
          {
            metricName: 'User Cls Failed Presubmit',
            metricDescription: 'User Cls Failed Presubmit Description',
            durationKey: '1d',
            thresholdValue: '10',
            actualValue: '0',
            satisfied: false,
          },
        ],
      },
      {
        priority: 'P2',
        satisfied: false,
        criteria: [
          {
            metricName: 'User Cls Failed Presubmit',
            metricDescription: 'User Cls Failed Presubmit Description',
            durationKey: '7d',
            thresholdValue: '1',
            actualValue: '0',
            satisfied: false,
          },
          {
            metricName: 'Presubmit-blocking Failures Exonerated',
            metricDescription: 'Critical Failures Exonerated Description',
            durationKey: '7d',
            thresholdValue: '1',
            actualValue: '0',
            satisfied: false,
          },
        ],
      },
    ]);
  });

  it('handles empty metric definitions', () => {
    // Empty metric definitions is not critical to making a recommendation;
    // it's just nice to have the human readable name for metrics.
    const r = createPriorityRecommendation(metricValues, [], priorities);
    expect(r).toEqual({
      recommendation: {
        priority: 'P0',
        satisfied: true,
        criteria: [
          {
            metricName: 'human-cls-failed-presubmit',
            metricDescription: '(metric description unavailable)',
            durationKey: '1d',
            thresholdValue: '20',
            actualValue: '98',
            satisfied: true,
          },
        ],
      },
      justification: [
        {
          priority: 'P0',
          satisfied: true,
          criteria: [
            {
              metricName: 'human-cls-failed-presubmit',
              metricDescription: '(metric description unavailable)',
              durationKey: '1d',
              thresholdValue: '20',
              actualValue: '98',
              satisfied: true,
            },
          ],
        },
        {
          priority: 'P1',
          satisfied: true,
          criteria: [
            {
              metricName: 'human-cls-failed-presubmit',
              metricDescription: '(metric description unavailable)',
              durationKey: '1d',
              thresholdValue: '10',
              actualValue: '98',
              satisfied: true,
            },
          ],
        },
        {
          priority: 'P2',
          satisfied: true,
          criteria: [
            {
              metricName: 'human-cls-failed-presubmit',
              metricDescription: '(metric description unavailable)',
              durationKey: '7d',
              thresholdValue: '1',
              actualValue: '167',
              satisfied: true,
            },
            {
              metricName: 'critical-failures-exonerated',
              metricDescription: '(metric description unavailable)',
              durationKey: '7d',
              thresholdValue: '1',
              actualValue: '0',
              satisfied: false,
            },
          ],
        },
      ],
    });
  });

  it('handles empty priority config', () => {
    const r = createPriorityRecommendation(metricValues, metrics, []);
    expect(r.recommendation).toBe(undefined);
    expect(r.justification).toHaveLength(0);
  });

  it('recommends the highest satisfied priority', () => {
    const r = createPriorityRecommendation(metricValues, metrics, priorities);
    expect(r).toEqual({
      recommendation: {
        priority: 'P0',
        satisfied: true,
        criteria: [
          {
            metricName: 'User Cls Failed Presubmit',
            metricDescription: 'User Cls Failed Presubmit Description',
            durationKey: '1d',
            thresholdValue: '20',
            actualValue: '98',
            satisfied: true,
          },
        ],
      },
      justification: [
        {
          priority: 'P0',
          satisfied: true,
          criteria: [
            {
              metricName: 'User Cls Failed Presubmit',
              metricDescription: 'User Cls Failed Presubmit Description',
              durationKey: '1d',
              thresholdValue: '20',
              actualValue: '98',
              satisfied: true,
            },
          ],
        },
        {
          priority: 'P1',
          satisfied: true,
          criteria: [
            {
              metricName: 'User Cls Failed Presubmit',
              metricDescription: 'User Cls Failed Presubmit Description',
              durationKey: '1d',
              thresholdValue: '10',
              actualValue: '98',
              satisfied: true,
            },
          ],
        },
        {
          priority: 'P2',
          satisfied: true,
          criteria: [
            {
              metricName: 'User Cls Failed Presubmit',
              metricDescription: 'User Cls Failed Presubmit Description',
              durationKey: '7d',
              thresholdValue: '1',
              actualValue: '167',
              satisfied: true,
            },
            {
              metricName: 'Presubmit-blocking Failures Exonerated',
              metricDescription: 'Critical Failures Exonerated Description',
              durationKey: '7d',
              thresholdValue: '1',
              actualValue: '0',
              satisfied: false,
            },
          ],
        },
      ],
    });
  });

  it('does not recommend a priority if the criteria has not been met', () => {
    const lowMetricValues: ClusterMetrics = {
      'human-cls-failed-presubmit': {
        oneDay: { nominal: '0' },
        threeDay: { nominal: '0' },
        sevenDay: { nominal: '0' },
      },
      'critical-failures-exonerated': {
        oneDay: { nominal: '0' },
        threeDay: { nominal: '0' },
        sevenDay: { nominal: '0' },
      },
      'failures': {
        oneDay: { nominal: '7625' },
        threeDay: { nominal: '16052' },
        sevenDay: { nominal: '15800' },
      },
    };
    const r = createPriorityRecommendation(lowMetricValues, metrics, priorities);
    expect(r.recommendation).toBe(undefined);
    expect(r.justification).toEqual([
      {
        priority: 'P0',
        satisfied: false,
        criteria: [
          {
            metricName: 'User Cls Failed Presubmit',
            metricDescription: 'User Cls Failed Presubmit Description',
            durationKey: '1d',
            thresholdValue: '20',
            actualValue: '0',
            satisfied: false,
          },
        ],
      },
      {
        priority: 'P1',
        satisfied: false,
        criteria: [
          {
            metricName: 'User Cls Failed Presubmit',
            metricDescription: 'User Cls Failed Presubmit Description',
            durationKey: '1d',
            thresholdValue: '10',
            actualValue: '0',
            satisfied: false,
          },
        ],
      },
      {
        priority: 'P2',
        satisfied: false,
        criteria: [
          {
            metricName: 'User Cls Failed Presubmit',
            metricDescription: 'User Cls Failed Presubmit Description',
            durationKey: '7d',
            thresholdValue: '1',
            actualValue: '0',
            satisfied: false,
          },
          {
            metricName: 'Presubmit-blocking Failures Exonerated',
            metricDescription: 'Critical Failures Exonerated Description',
            durationKey: '7d',
            thresholdValue: '1',
            actualValue: '0',
            satisfied: false,
          },
        ],
      },
    ]);
  });


  it('does not recommend a priority if the criteria for lower priorities has not been met', () => {
    // The below metrics do not conceptually make sense, but are based on
    // specifically satisfying some priority criteria but not others.
    const trickMetricValues: ClusterMetrics = {
      'human-cls-failed-presubmit': {
        oneDay: { nominal: '27' },
        threeDay: { nominal: '0' },
        sevenDay: { nominal: '0' },
      },
      'critical-failures-exonerated': {
        oneDay: { nominal: '0' },
        threeDay: { nominal: '0' },
        sevenDay: { nominal: '0' },
      },
      'failures': {
        oneDay: { nominal: '7625' },
        threeDay: { nominal: '16052' },
        sevenDay: { nominal: '15800' },
      },
    };
    const r = createPriorityRecommendation(trickMetricValues, metrics, priorities);
    expect(r.recommendation).toBe(undefined);
    expect(r.justification).toEqual([
      {
        priority: 'P0',
        satisfied: false,
        criteria: [
          {
            metricName: 'User Cls Failed Presubmit',
            metricDescription: 'User Cls Failed Presubmit Description',
            durationKey: '1d',
            thresholdValue: '20',
            actualValue: '27',
            satisfied: true,
          },
        ],
      },
      {
        priority: 'P1',
        satisfied: false,
        criteria: [
          {
            metricName: 'User Cls Failed Presubmit',
            metricDescription: 'User Cls Failed Presubmit Description',
            durationKey: '1d',
            thresholdValue: '10',
            actualValue: '27',
            satisfied: true,
          },
        ],
      },
      {
        priority: 'P2',
        satisfied: false,
        criteria: [
          {
            metricName: 'User Cls Failed Presubmit',
            metricDescription: 'User Cls Failed Presubmit Description',
            durationKey: '7d',
            thresholdValue: '1',
            actualValue: '0',
            satisfied: false,
          },
          {
            metricName: 'Presubmit-blocking Failures Exonerated',
            metricDescription: 'Critical Failures Exonerated Description',
            durationKey: '7d',
            thresholdValue: '1',
            actualValue: '0',
            satisfied: false,
          },
        ],
      },
    ]);
  });
});
