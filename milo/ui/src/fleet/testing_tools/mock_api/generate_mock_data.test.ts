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

import * as fs from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';

import {
  generateChromeOSDevices,
  generateAndroidDevices,
  generateBrowserDevices,
  generateProductCatalog,
  generateRepairMetrics,
  generateResourceRequests,
  createPrng,
  resetSeed,
  writeSafeJson,
  getDefaultDataDir,
  runGenerator,
  DETERMINISTIC_BASE_TIMESTAMP_MS,
  FICTIONAL_BOARDS,
  FICTIONAL_MODELS,
  FICTIONAL_LABS,
} from '../../scripts/generate_mock_data';

describe('Fleet Console Mock Data Generator Pipeline', () => {
  describe('Synthetic Data Structure Validation', () => {
    it('produces ChromeOS mock data using only fictional dictionaries', () => {
      const devices = generateChromeOSDevices(50);
      for (const dev of devices) {
        const board = dev.deviceSpec.labels.board?.values[0];
        const model = dev.deviceSpec.labels.model?.values[0];

        const isBoardValid =
          !board ||
          (FICTIONAL_BOARDS as readonly string[]).includes(board) ||
          board === 'mock-board-labstation' ||
          board === 'mock-board-satlab';
        expect(isBoardValid).toBe(true);

        const isModelValid =
          !model ||
          (FICTIONAL_MODELS as readonly string[]).includes(model) ||
          model === 'sim-model-labstation' ||
          model === 'sim-model-satlab';
        expect(isModelValid).toBe(true);
      }
    });

    it('produces Android mock data using only fictional dictionaries', () => {
      const devices = generateAndroidDevices(50);
      for (const dev of devices) {
        expect(dev.runTarget).toMatch(/^(demo|mock|sim)-target-/);
      }
    });

    it('produces Browser mock data using only fictional dictionaries', () => {
      const devices = generateBrowserDevices(50);
      for (const dev of devices) {
        expect(dev.realm).toMatch(/^(demo|mock|sim)/);
      }
    });

    it('produces Repair Metrics and Resource Requests with synthetic demo prefixes', () => {
      const repairs = generateRepairMetrics(15);
      for (const repair of repairs) {
        expect(repair.labName).toMatch(/^(demo|mock|sim)-lab-/);
      }
      const requests = generateResourceRequests(15);
      for (const req of requests) {
        expect(req.rrId).toMatch(/^RR-\d{4}-\d{3}$/);
      }
    });
  });

  describe('PRNG Determinism and State Safety', () => {
    it('produces strictly identical numbers for Mulberry32 with matching seeds', () => {
      const prng1 = createPrng(42);
      const prng2 = createPrng(42);

      const seq1 = Array.from({ length: 20 }, () => prng1());
      const seq2 = Array.from({ length: 20 }, () => prng2());

      expect(seq1).toEqual(seq2);
    });

    it('produces bit-for-bit identical datasets after resetSeed', () => {
      resetSeed(100);
      const run1 = JSON.stringify(generateAndroidDevices(10));

      resetSeed(100);
      const run2 = JSON.stringify(generateAndroidDevices(10));

      expect(run1).toBe(run2);
    });

    it('uses a deterministic base timestamp instead of Date.now() for offline timestamps', () => {
      resetSeed(42);
      const devices = generateAndroidDevices(20);
      const offlineDev = devices.find((d) => d.fcOfflineSince !== undefined);

      expect(offlineDev).toBeDefined();
      expect(offlineDev?.fcOfflineSince).toMatch(/^2026-09-01T/);
      expect(
        new Date(offlineDev?.fcOfflineSince ?? '').getTime(),
      ).toBeLessThanOrEqual(DETERMINISTIC_BASE_TIMESTAMP_MS);
    });
  });

  describe('Path Traversal Confinement (CWE-22)', () => {
    it('rejects filenames attempting directory traversal with ..', () => {
      const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'fcon-test-'));
      try {
        expect(() => {
          writeSafeJson(tmpDir, '../evil.json', { test: true });
        }).toThrow(/Path traversal attempt/);
      } finally {
        fs.rmSync(tmpDir, { recursive: true, force: true });
      }
    });

    it('rejects filenames with directory separators / or \\', () => {
      const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'fcon-test-'));
      try {
        expect(() => {
          writeSafeJson(tmpDir, 'sub/evil.json', { test: true });
        }).toThrow(/Path traversal attempt/);
        expect(() => {
          writeSafeJson(tmpDir, 'sub\\evil.json', { test: true });
        }).toThrow(/Path traversal attempt/);
      } finally {
        fs.rmSync(tmpDir, { recursive: true, force: true });
      }
    });

    it('successfully writes valid filenames strictly within the target directory', () => {
      const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'fcon-test-'));
      try {
        const out = writeSafeJson(tmpDir, 'safe_file.json', { safe: true });
        expect(fs.existsSync(out)).toBe(true);
        const parsed = JSON.parse(fs.readFileSync(out, 'utf-8'));
        expect(parsed).toEqual({ safe: true });
      } finally {
        fs.rmSync(tmpDir, { recursive: true, force: true });
      }
    });
  });

  describe('Obvious Demo Indicators', () => {
    it('ensures all boards, models, and labs have explicit mock/demo/sim prefixes', () => {
      for (const board of FICTIONAL_BOARDS) {
        expect(board).toMatch(/^(demo|mock|sim)-board-/);
      }
      for (const model of FICTIONAL_MODELS) {
        expect(model).toMatch(/^(demo|mock|sim)-model-/);
      }
      for (const lab of FICTIONAL_LABS) {
        expect(lab.name).toMatch(/^(demo|mock|sim)-lab-/);
        expect(lab.zone).toMatch(/^ZONE_(DEMO|MOCK|SIM)_/);
      }
    });

    it('assigns obvious demo prefixes to device hostnames and IDs', () => {
      const cros = generateChromeOSDevices(10);
      for (const dev of cros) {
        expect(dev.id).toMatch(/^demo-cros-/);
        expect(dev.dutId).toMatch(/^DEMO-(DUT|LABSTATION|SATLAB)-/);
        expect(dev.realm).toMatch(/^demo:fleet\/mock-realm-(cros|satlab)$/);
      }

      const android = generateAndroidDevices(10);
      for (const dev of android) {
        expect(dev.id).toMatch(/^DEMO-AND-/);
        expect(dev.omnilabSpec.labels.hostname.values[0]).toMatch(
          /^sim-android-host-/,
        );
      }

      const browser = generateBrowserDevices(10);
      for (const dev of browser) {
        expect(dev.id).toMatch(/^demo-bot-swarm-/);
        expect(dev.realm).toBe('demo:fleet/mock-realm-browser');
      }
    });
  });

  describe('Full Schema and Column Coverage', () => {
    it('populates all standard ChromeOS labels and states', () => {
      const [dev] = generateChromeOSDevices(1);
      const labels = dev.deviceSpec.labels;

      expect(dev.state).toMatch(/^DEVICE_STATE_/);
      expect(labels['label-board'].values[0]).toBeDefined();
      expect(labels['label-model'].values[0]).toBeDefined();
      expect(labels['label-pool'].values[0]).toBeDefined();
      expect(labels['label-phase'].values[0]).toBeDefined();
      expect(labels['label-servo_state'].values[0]).toBeDefined();
      expect(labels['label-servo_component'].values.length).toBeGreaterThan(0);
      expect(labels['dut_state'].values[0]).toBeDefined();
      expect(labels['ufs_zone'].values[0]).toBeDefined();
    });

    it('models ChromeOS DUT and labstation topologies with relational fidelity', () => {
      const devices = generateChromeOSDevices(20);
      const labstation = devices.find((d) =>
        d.dutId.startsWith('DEMO-LABSTATION-'),
      );
      const dut = devices.find((d) => d.dutId.startsWith('DEMO-DUT-'));

      expect(labstation).toBeDefined();
      expect(dut).toBeDefined();

      // Labstations have managed DUTs and no servo attachments
      expect(
        labstation?.deviceSpec?.labels['label-managed_dut']?.values.length,
      ).toBeGreaterThan(0);
      expect(
        labstation?.deviceSpec?.labels['label-servo_state'],
      ).toBeUndefined();

      // DUTs have servo attachments and associated labstation hostname
      expect(
        dut?.deviceSpec?.labels['label-servo_state']?.values[0],
      ).toBeDefined();
      expect(
        dut?.deviceSpec?.labels['label-associated_hostname']?.values[0],
      ).toMatch(/^demo-cros-labstation-/);
    });

    it('populates all standard Android labels and states', () => {
      const [dev] = generateAndroidDevices(1);
      const labels = dev.omnilabSpec.labels;

      expect(labels.state.values[0]).toBeDefined();
      expect(labels.dut_state.values[0]).toBeDefined();
      expect(labels.host_ip.values[0]).toMatch(/^10\./);
      expect(labels.model.values[0]).toBeDefined();
      expect(labels.run_target.values[0]).toBeDefined();
      expect(labels.host_group.values[0]).toBeDefined();
      expect(labels.lab_name.values[0]).toBeDefined();
      expect(labels.health_category.values[0]).toBeDefined();
      expect(labels.fc_is_offline.values[0]).toMatch(/^(true|false)$/);
      expect(dev.average7d).toBeGreaterThan(0);
    });

    it('populates all standard Browser swarming and UFS labels', () => {
      const [dev] = generateBrowserDevices(1);

      expect(dev.swarmingLabels.os.values[0]).toBeDefined();
      expect(dev.swarmingLabels.pool.values[0]).toBeDefined();
      expect(dev.swarmingLabels.state.values[0]).toBeDefined();
      expect(dev.swarmingLabels.current_task.values[0]).toBeDefined();
      expect(dev.swarmingLabels.last_seen.values[0]).toBeDefined();
      expect(dev.swarmingLabels.gpu.values[0]).toBeDefined();
      expect(dev.ufsLabels.hostname.values[0]).toBeDefined();
      expect(dev.ufsLabels.model.values[0]).toBeDefined();
      expect(dev.ufsLabels.rack.values[0]).toBeDefined();
      expect(dev.ufsLabels.zone.values[0]).toBeDefined();
      expect(['SERVING', 'REGISTERED']).toContain(
        dev.ufsLabels.resource_state.values[0],
      );
    });

    it('populates Product Catalog, Repair Metrics, and Resource Requests with relational fidelity', () => {
      const catalog = generateProductCatalog(5);
      expect(catalog).toHaveLength(5);
      expect(catalog[0].productCatalogId).toMatch(/^PROD-CAT-/);
      expect(catalog[0].unitCost).toMatch(/^\$\d+/);
      expect(
        (catalog[0] as unknown as Record<string, unknown>).id,
      ).toBeUndefined();

      const repairs = generateRepairMetrics(5);
      expect(repairs).toHaveLength(5);
      expect(repairs[0].priority).toMatch(/^(BREACHED|WATCH|NICE)$/);
      expect(repairs[0].totalDevices).toBeGreaterThan(0);
      expect(
        (repairs[0] as unknown as Record<string, unknown>).id,
      ).toBeUndefined();
      expect(
        (repairs[0] as unknown as Record<string, unknown>).platform,
      ).toBeUndefined();

      const requests = generateResourceRequests(5);
      expect(requests).toHaveLength(5);
      expect(requests[0].rrId).toMatch(/^RR-2026-/);
      expect(requests[0].acceptedQuantity).toBeGreaterThan(0);
      expect(Array.isArray(requests[0].resourceGroups)).toBe(true);
    });
  });

  describe('Generator Pipeline Execution & Edge Cases', () => {
    it('gracefully handles count <= 0 for all generator functions', () => {
      expect(generateChromeOSDevices(0)).toEqual([]);
      expect(generateChromeOSDevices(-5)).toEqual([]);
      expect(generateAndroidDevices(0)).toEqual([]);
      expect(generateAndroidDevices(-5)).toEqual([]);
      expect(generateBrowserDevices(0)).toEqual([]);
      expect(generateBrowserDevices(-5)).toEqual([]);
      expect(generateProductCatalog(0)).toEqual([]);
      expect(generateProductCatalog(-5)).toEqual([]);
      expect(generateRepairMetrics(0)).toEqual([]);
      expect(generateRepairMetrics(-5)).toEqual([]);
      expect(generateResourceRequests(0)).toEqual([]);
      expect(generateResourceRequests(-5)).toEqual([]);
    });

    it('successfully runs generator pipeline to a custom output directory', () => {
      const tmpDir = fs.mkdtempSync(
        path.join(os.tmpdir(), 'fcon-pipeline-test-'),
      );
      try {
        const outDir = runGenerator({ outputDir: tmpDir, isFull: false });
        expect(outDir).toBe(path.resolve(tmpDir));

        const files = [
          'chromeos_devices.json',
          'android_devices.json',
          'browser_devices.json',
          'product_catalog.json',
          'repair_metrics.json',
          'resource_requests.json',
        ];

        for (const file of files) {
          const filePath = path.join(tmpDir, file);
          expect(fs.existsSync(filePath)).toBe(true);
          const content = JSON.parse(fs.readFileSync(filePath, 'utf-8'));
          expect(Array.isArray(content)).toBe(true);
          expect(content.length).toBeGreaterThan(0);
        }
      } finally {
        fs.rmSync(tmpDir, { recursive: true, force: true });
      }
    });

    it('resolves default data directory relative to repository layout', () => {
      const defaultDir = getDefaultDataDir();
      expect(defaultDir).toContain('testing_tools');
      expect(defaultDir).toContain('mock_api');
      expect(defaultDir).toContain('data');
    });

    it('asserts checked-in JSON sample fixtures in source control match generator output bit-for-bit', () => {
      const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'fcon-sync-check-'));
      try {
        runGenerator({ outputDir: tmpDir, isFull: false });
        const defaultDir = getDefaultDataDir();

        const files = [
          'chromeos_devices.json',
          'android_devices.json',
          'browser_devices.json',
          'product_catalog.json',
          'repair_metrics.json',
          'resource_requests.json',
        ];

        for (const file of files) {
          const generatedContent = fs.readFileSync(
            path.join(tmpDir, file),
            'utf-8',
          );
          const checkedInContent = fs.readFileSync(
            path.join(defaultDir, file),
            'utf-8',
          );
          expect(generatedContent).toBe(checkedInContent);
        }
      } finally {
        fs.rmSync(tmpDir, { recursive: true, force: true });
      }
    });
  });
});
