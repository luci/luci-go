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

import { escapeMarkdown, md, rawMd } from './markdown_utils';

describe('markdown_utils', () => {
  describe('md tagged template literal', () => {
    it('safely escapes dynamic expressions inside markdown templates', () => {
      const devId = 'chrome-win10-001';
      const devStatus = 'ready](https://evil.example/flops-sso) [x';
      const url = rawMd('https://ci.chromium.org/ui/fleet/devices');

      const line = md`* **Machine:** [${devId}](${url}) - Status: ${devStatus}`;
      expect(line).toBe(
        '* **Machine:** [chrome\\-win10\\-001](https://ci.chromium.org/ui/fleet/devices) - Status: ready\\]\\(https://evil\\.example/flops\\-sso\\) \\[x',
      );
    });

    it('handles undefined and null interpolations gracefully', () => {
      const undef = undefined;
      const nul = null;
      expect(md`prefix ${undef} middle ${nul} suffix`).toBe(
        'prefix  middle  suffix',
      );
    });

    it('allows trusted raw Markdown using rawMd() without double-escaping', () => {
      const untrusted = '*bold*';
      const trusted = rawMd('`already safe`');
      const result = md`Untrusted: ${untrusted}, Trusted: ${trusted}`;
      expect(result).toBe('Untrusted: \\*bold\\*, Trusted: `already safe`');
    });

    it('handles numeric and boolean values', () => {
      const count = 42;
      const enabled = true;
      expect(md`Count: ${count}, Enabled: ${enabled}`).toBe(
        'Count: 42, Enabled: true',
      );
    });
  });

  describe('escapeMarkdown', () => {
    it('returns empty string for undefined, null, or empty string', () => {
      expect(escapeMarkdown(undefined)).toBe('');
      expect(escapeMarkdown('')).toBe('');
    });

    it('escapes Markdown link delimiters and prevents link injection', () => {
      const malicious = 'ready](https://evil.example/flops-sso) [x';
      const escaped = escapeMarkdown(malicious);
      expect(escaped).toBe(
        'ready\\]\\(https://evil\\.example/flops\\-sso\\) \\[x',
      );
    });

    it('escapes formatting metacharacters (asterisks, underscores, backticks, tildes, hashes)', () => {
      const text = '*bold* _italic_ `code` ~strike~ # heading';
      const escaped = escapeMarkdown(text);
      expect(escaped).toBe(
        '\\*bold\\* \\_italic\\_ \\`code\\` \\~strike\\~ \\# heading',
      );
    });

    it('escapes list punctuation (+, -, .) to prevent accidental list injection', () => {
      const text = '+ item 1 - item 2 1. ordered item';
      const escaped = escapeMarkdown(text);
      expect(escaped).toBe('\\+ item 1 \\- item 2 1\\. ordered item');
    });

    it('escapes brackets, braces, and angles', () => {
      const text = '[link] {block} <tag>';
      const escaped = escapeMarkdown(text);
      expect(escaped).toBe('\\[link\\] \\{block\\} \\<tag\\>');
    });

    it('escapes backslashes first', () => {
      const text = 'path\\to\\file';
      const escaped = escapeMarkdown(text);
      expect(escaped).toBe('path\\\\to\\\\file');
    });

    it('normalizes newlines to spaces to prevent line and block injection', () => {
      const multiline = 'line1\n* **ACTION REQUIRED:** evil payload\r\nline2';
      const escaped = escapeMarkdown(multiline);
      expect(escaped).toBe(
        'line1 \\* \\*\\*ACTION REQUIRED:\\*\\* evil payload line2',
      );
      expect(escaped).not.toContain('\n');
      expect(escaped).not.toContain('\r');
    });
  });

  describe('Markdown tables with md', () => {
    it('safely escapes table cells including pipes, backslashes, and newlines', () => {
      const host = 'host1 | evil_column';
      const serial = 'SN-123\nnewline';
      const status = 'broken | [link](evil)';
      const row = md`| ${host} | ${serial} | ${status} |`;
      expect(row).toBe(
        '| host1 \\| evil\\_column | SN\\-123 newline | broken \\| \\[link\\]\\(evil\\) |',
      );
    });

    it('supports nested links in table cells using rawMd', () => {
      const devId = 'chrome-win10-001';
      const url = rawMd('https://ci.chromium.org/ui/fleet/devices/123');
      const status = 'ready';
      const machineCell = rawMd(md`[${devId}](${url})`);
      const row = md`| ${machineCell} | ${status} |`;
      expect(row).toBe(
        '| [chrome\\-win10\\-001](https://ci.chromium.org/ui/fleet/devices/123) | ready |',
      );
    });
  });
});
