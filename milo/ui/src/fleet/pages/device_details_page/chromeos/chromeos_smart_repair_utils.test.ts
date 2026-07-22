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

import {
  convertGsToHttp,
  formatLogTimestamp,
  getConclusionIcon,
  getConclusionSeverity,
  getHeaderStatusLabel,
  getNormalizedResult,
} from './chromeos_smart_repair_utils';

describe('chromeos_smart_repair_utils', () => {
  describe('getConclusionSeverity', () => {
    it('should return error for fail, broken, error status', () => {
      expect(getConclusionSeverity('failed')).toBe('error');
      expect(getConclusionSeverity('broken_dut')).toBe('error');
      expect(getConclusionSeverity('some_error')).toBe('error');
    });

    it('should return warning for warn status', () => {
      expect(getConclusionSeverity('warning')).toBe('warning');
      expect(getConclusionSeverity('warn')).toBe('warning');
    });

    it('should return success for pass, success, work status', () => {
      expect(getConclusionSeverity('passed')).toBe('success');
      expect(getConclusionSeverity('success')).toBe('success');
      expect(getConclusionSeverity('working')).toBe('success');
    });

    it('should return info for other/undefined status', () => {
      expect(getConclusionSeverity('unknown')).toBe('info');
      expect(getConclusionSeverity(undefined)).toBe('info');
    });
  });

  describe('getHeaderStatusLabel', () => {
    it('should map to Success for pass/success status', () => {
      expect(getHeaderStatusLabel('passed')).toBe('Success');
      expect(getHeaderStatusLabel('Success')).toBe('Success');
    });

    it('should map to Failure for fail/error status', () => {
      expect(getHeaderStatusLabel('failed')).toBe('Failure');
      expect(getHeaderStatusLabel('Failed: SBU_LOW_VOLTAGE')).toBe('Failure');
      expect(getHeaderStatusLabel('broken_dut')).toBe('Failure');
    });

    it('should map to Warning for warn status', () => {
      expect(getHeaderStatusLabel('warning')).toBe('Warning');
      expect(getHeaderStatusLabel('warn')).toBe('Warning');
    });

    it('should map to Info for other/undefined status', () => {
      expect(getHeaderStatusLabel('unknown')).toBe('Info');
      expect(getHeaderStatusLabel(undefined)).toBe('Info');
    });
  });

  describe('getConclusionIcon', () => {
    it('should return correct icon for each severity', () => {
      expect(getConclusionIcon('error')).toBe('!');
      expect(getConclusionIcon('warning')).toBe('⚠');
      expect(getConclusionIcon('success')).toBe('✓');
      expect(getConclusionIcon('info')).toBe('?');
    });
  });

  describe('formatLogTimestamp', () => {
    it('should format valid embedded timestamp to UTC format', () => {
      const logsPath =
        'gs://chromeos-autotest-results/12345-repair/2026-05-18-15-30-45/autoserv.DEBUG';
      expect(formatLogTimestamp(logsPath)).toBe(
        'Mon, 18 May 2026 15:30:45 UTC',
      );
    });

    it('should return null for invalid or missing timestamps', () => {
      expect(
        formatLogTimestamp(
          'gs://chromeos-autotest-results/12345-repair/no-timestamp/autoserv.DEBUG',
        ),
      ).toBeNull();
      expect(formatLogTimestamp('')).toBeNull();
    });
  });

  describe('convertGsToHttp', () => {
    it('should convert gs:// links to Google Cloud Storage Console links', () => {
      const gsPath =
        'gs://chromeos-autotest-results/12345-repair/2026-05-18-15-30-45';
      expect(convertGsToHttp(gsPath)).toBe(
        'https://console.cloud.google.com/storage/browser/chromeos-autotest-results/12345-repair/2026-05-18-15-30-45',
      );
    });

    it('should allow trusted Google Cloud/Chromium HTTPS URLs', () => {
      const trustedPath =
        'https://console.cloud.google.com/storage/browser/logs';
      expect(convertGsToHttp(trustedPath)).toBe(trustedPath);
      expect(convertGsToHttp('https://storage.googleapis.com/bucket/log')).toBe(
        'https://storage.googleapis.com/bucket/log',
      );
    });

    it('should reject untrusted or external URLs', () => {
      const httpPath = 'https://evil.example.com/log';
      expect(convertGsToHttp(httpPath)).toBe('');
      expect(convertGsToHttp('')).toBe('');
    });
  });

  describe('getNormalizedResult', () => {
    it('should parse standard raw results object', () => {
      const raw = {
        summary: 'Device is fully healthy.',
        logsPath: 'gs://bucket/logs',
        manualRepairActions: ['Replace charger', 'Reboot device'],
        conclusions: [
          {
            target: 'Power',
            status: 'Passed',
            summary: 'Battery is good.',
            recoveries: ['Charged battery'],
          },
        ],
      };

      const normalized = getNormalizedResult(raw);
      expect(normalized.summary).toBe('Device is fully healthy.');
      expect(normalized.logsPath).toBe('gs://bucket/logs');
      expect(normalized.manualRepairActions).toEqual([
        'Replace charger',
        'Reboot device',
      ]);
      expect(normalized.conclusions).toEqual([
        {
          target: 'Power',
          status: 'Passed',
          summary: 'Battery is good.',
          recoveries: ['Charged battery'],
        },
      ]);
    });

    it('should parse stringified JSON payloads nested inside summary', () => {
      const nestedResult = {
        summary: 'Nested Summary',
        logsPath: 'gs://bucket/nested-logs',
        manualRepairActions: ['Perform powerwash'],
        conclusions: [
          {
            target: 'TPM',
            status: 'Failed',
            summary: 'TPM cannot be initialized',
            recoveries: [],
          },
        ],
      };

      const raw = {
        summary: JSON.stringify({ result: nestedResult }),
      };

      const normalized = getNormalizedResult(raw);
      expect(normalized.summary).toBe('Nested Summary');
      expect(normalized.logsPath).toBe('gs://bucket/nested-logs');
      expect(normalized.manualRepairActions).toEqual(['Perform powerwash']);
      expect(normalized.conclusions).toEqual([
        {
          target: 'TPM',
          status: 'Failed',
          summary: 'TPM cannot be initialized',
          recoveries: [],
        },
      ]);
    });

    it('should handle conclusions structured as raw strings', () => {
      const raw = {
        conclusions: [
          'Battery: Passed - Battery level is 95%',
          'OS: Failed - OS corrupt',
          'Network: No network connection',
        ],
      };

      const normalized = getNormalizedResult(raw);
      expect(normalized.conclusions).toEqual([
        {
          target: 'Battery',
          status: 'Passed',
          summary: 'Battery level is 95%',
          recoveries: [],
        },
        {
          target: 'OS',
          status: 'Failed',
          summary: 'OS corrupt',
          recoveries: [],
        },
        {
          target: 'Network',
          status: 'Unknown',
          summary: 'No network connection',
          recoveries: [],
        },
      ]);
    });

    it('should return empty normalized structure for null/undefined raw results', () => {
      const expected = {
        summary: '',
        conclusions: [],
        manualRepairActions: [],
        logsPath: '',
      };
      expect(getNormalizedResult(null)).toEqual(expected);
      expect(getNormalizedResult(undefined)).toEqual(expected);
    });
  });
});
