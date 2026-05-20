// Copyright 2026 The LUCI Authors.
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

import { Timestamp } from 'firebase/firestore';

export interface SmartRepairRealtimeData {
  dut_id?: string;
  event_id?: string;
  status?: 'pending' | 'processing' | 'completed' | 'error';
  requestTimestamp?: Timestamp;
  result?: {
    summary?: string;
    conclusions?: readonly {
      target?: string;
      status?: string;
      summary?: string;
      recoveries?: readonly string[];
    }[];
    manualRepairActions?: readonly string[];
    logsPath?: string;
  };
  error?: {
    message?: string;
  };
  completionTimestamp?: Timestamp;
}

export interface NormalizedConclusion {
  target: string;
  status: string;
  summary: string;
  recoveries: string[];
}

export interface NormalizedSmartRepairResult {
  summary: string;
  conclusions: NormalizedConclusion[];
  manualRepairActions: string[];
  logsPath: string;
}

export interface RawSmartRepairResult {
  summary?: string;
  logsPath?: string;
  logs_path?: string;
  manualRepairActions?: readonly string[];
  manual_repair_actions?: readonly string[];
  conclusions?: readonly unknown[];
  conclusion?: readonly unknown[];
}

export const getConclusionSeverity = (
  status: string | undefined,
): 'error' | 'warning' | 'success' | 'info' => {
  const lower = status?.toLowerCase() || '';
  if (
    lower.includes('fail') ||
    lower.includes('broken') ||
    lower.includes('error')
  ) {
    return 'error';
  }
  if (lower.includes('warn')) {
    return 'warning';
  }
  if (
    lower.includes('pass') ||
    lower.includes('success') ||
    lower.includes('work')
  ) {
    return 'success';
  }
  return 'info';
};

export const getHeaderStatusLabel = (status: string | undefined): string => {
  const severity = getConclusionSeverity(status);
  switch (severity) {
    case 'success':
      return 'Success';
    case 'error':
      return 'Failure';
    case 'warning':
      return 'Warning';
    default:
      return 'Info';
  }
};

export const getConclusionIcon = (
  severity: 'error' | 'warning' | 'success' | 'info',
): string => {
  switch (severity) {
    case 'error':
      return '!';
    case 'warning':
      return '⚠';
    case 'success':
      return '✓';
    default:
      return '?';
  }
};

export const getNormalizedResult = (
  rawResult: RawSmartRepairResult | null | undefined,
): NormalizedSmartRepairResult => {
  const normalized: NormalizedSmartRepairResult = {
    summary: '',
    conclusions: [],
    manualRepairActions: [],
    logsPath: '',
  };

  if (!rawResult) {
    return normalized;
  }

  // 1. Check if summary is a JSON string containing the actual data
  let source: RawSmartRepairResult = rawResult;
  if (
    typeof rawResult.summary === 'string' &&
    rawResult.summary.trim().startsWith('{')
  ) {
    try {
      const parsed = JSON.parse(rawResult.summary) as RawSmartRepairResult & {
        result?: RawSmartRepairResult;
      };
      if (parsed) {
        source = parsed.result || parsed;
      }
    } catch {
      // Fallback to using rawResult as is
    }
  }

  // 2. Extract summary
  normalized.summary =
    typeof source.summary === 'string'
      ? source.summary
      : rawResult.summary || '';

  // 3. Extract logsPath
  normalized.logsPath =
    source.logsPath ||
    source.logs_path ||
    rawResult.logsPath ||
    rawResult.logs_path ||
    '';

  // 4. Extract manualRepairActions
  const rawActions =
    source.manualRepairActions ||
    source.manual_repair_actions ||
    rawResult.manualRepairActions ||
    rawResult.manual_repair_actions ||
    [];
  if (Array.isArray(rawActions)) {
    normalized.manualRepairActions = rawActions.map((act: unknown) =>
      String(act),
    );
  }

  // 5. Extract conclusions and normalize them
  const rawConclusions = source.conclusions || rawResult.conclusions || [];
  if (Array.isArray(rawConclusions)) {
    normalized.conclusions = rawConclusions.map((c: unknown) => {
      if (typeof c === 'string') {
        // Format: "Target: Status - Summary" or "Target - Status: Summary" or just "Target: Summary"
        let target = 'Unknown';
        let status = 'Unknown';
        let summary = c;

        const colonIndex = c.indexOf(':');
        if (colonIndex !== -1) {
          target = c.substring(0, colonIndex).trim();
          const rest = c.substring(colonIndex + 1).trim();

          const dashIndex = rest.indexOf('-');
          if (dashIndex !== -1) {
            status = rest.substring(0, dashIndex).trim();
            summary = rest.substring(dashIndex + 1).trim();
          } else {
            // Check if first word is a status
            const words = rest.split(/\s+/);
            if (
              words.length > 0 &&
              [
                'passed',
                'failed',
                'success',
                'error',
                'warning',
                'broken',
                'working',
              ].includes(words[0].toLowerCase().replace(/[^a-z]/gi, ''))
            ) {
              status = words[0];
              summary = rest.substring(words[0].length).trim();
            } else {
              summary = rest;
            }
          }
        } else {
          const dashIndex = c.indexOf('-');
          if (dashIndex !== -1) {
            target = c.substring(0, dashIndex).trim();
            summary = c.substring(dashIndex + 1).trim();
          }
        }

        return {
          target,
          status,
          summary,
          recoveries: [],
        };
      } else if (c && typeof c === 'object') {
        const obj = c as Record<string, unknown>;
        return {
          target: String(obj.target || 'Unknown'),
          status: String(obj.status || 'Unknown'),
          summary: String(obj.summary || ''),
          recoveries: Array.isArray(obj.recoveries)
            ? obj.recoveries.map(String)
            : [],
        };
      }
      return {
        target: 'Unknown',
        status: 'Unknown',
        summary: String(c),
        recoveries: [],
      };
    });
  }

  return normalized;
};

export const formatLogTimestamp = (logsPath: string): string | null => {
  if (!logsPath) return null;
  const match = logsPath.match(/(\d{4}-\d{2}-\d{2}-\d{2}-\d{2}-\d{2})/);
  if (!match) return null;

  const parts = match[1].split('-');
  try {
    const date = new Date(
      Date.UTC(
        parseInt(parts[0], 10),
        parseInt(parts[1], 10) - 1,
        parseInt(parts[2], 10),
        parseInt(parts[3], 10),
        parseInt(parts[4], 10),
        parseInt(parts[5], 10),
      ),
    );

    if (isNaN(date.getTime())) return null;

    return date.toUTCString().replace('GMT', 'UTC');
  } catch {
    return null;
  }
};

export const convertGsToHttp = (gsPath: string): string => {
  if (!gsPath) return '';
  if (gsPath.startsWith('gs://')) {
    return (
      'https://console.cloud.google.com/storage/browser/' + gsPath.substring(5)
    );
  }
  return gsPath;
};
