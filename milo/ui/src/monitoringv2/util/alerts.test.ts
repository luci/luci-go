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
  BuilderAlertBuilder,
  groupAlerts,
  StepAlertBuilder,
  TestAlertBuilder,
} from '../components/testing_tools/test_utils';

import { AlertOrganizer, AlertOrganzerOptions } from './alerts';

describe('AlertOrganizer', () => {
  const hideAll: AlertOrganzerOptions = {
    showResolved: false,
    showFlaky: false,
    showChildrenHidden: false,
  };
  describe('allAlerts', () => {
    it('should return all alerts of a specific kind, including grouped alerts', () => {
      const alerts = [
        new BuilderAlertBuilder().build(),
        new StepAlertBuilder().build(),
        new TestAlertBuilder().build(),
      ];
      const groups = groupAlerts([alerts]);

      const organizer = new AlertOrganizer(alerts, groups, hideAll);

      expect(organizer.allAlerts('builder').length).toBe(1);
      expect(organizer.allAlerts('step').length).toBe(1);
      expect(organizer.allAlerts('test').length).toBe(1);
    });
    it('should show an ungrouped alert even if all of its grandchildren are grouped', () => {
      const alerts = [
        new BuilderAlertBuilder().build(),
        new StepAlertBuilder().build(),
        new TestAlertBuilder().build(),
      ];
      const groups = groupAlerts([[alerts[2]]]);

      const organizer = new AlertOrganizer(alerts, groups, hideAll);

      expect(organizer.allAlerts('builder').length).toBe(1);
    });
    it('should only group tests under a step and not also directly under a builder if there is a step associated with the test', () => {
      const alerts = [
        new BuilderAlertBuilder().build(),
        new StepAlertBuilder().build(),
        new TestAlertBuilder().build(),
      ];

      const organizer = new AlertOrganizer(alerts, {}, hideAll);
      const structured = organizer.allAlerts('builder');

      expect(structured.length).toBe(1);
      expect(structured[0].children.length).toBe(1);
      expect(structured[0].children[0].alert.kind).toBe('step');
      expect(structured[0].children[0].children.length).toBe(1);
      expect(structured[0].children[0].children[0].alert.kind).toBe('test');
    });
    it('should group tests directly under a builder if there is no step associated with the test', () => {
      const alerts = [
        new BuilderAlertBuilder().build(),
        new TestAlertBuilder().build(),
      ];

      const organizer = new AlertOrganizer(alerts, {}, hideAll);
      const structured = organizer.allAlerts('builder');

      expect(structured.length).toBe(1);
      expect(structured[0].children.length).toBe(1);
      expect(structured[0].children[0].alert.kind).toBe('test');
    });
  });
  describe('ungroupedAlerts', () => {
    it('should return all alerts of a specific kind when there are no grouped alerts', () => {
      const alerts = [
        new BuilderAlertBuilder().build(),
        new StepAlertBuilder().build(),
        new TestAlertBuilder().build(),
      ];

      const organizer = new AlertOrganizer(alerts, {}, hideAll);

      expect(organizer.ungroupedAlerts('builder').length).toBe(1);
      expect(organizer.ungroupedAlerts('step').length).toBe(1);
      expect(organizer.ungroupedAlerts('test').length).toBe(1);
    });
    it('should return no alerts of a specific kind when all alerts are grouped', () => {
      const alerts = [
        new BuilderAlertBuilder().build(),
        new StepAlertBuilder().build(),
        new TestAlertBuilder().build(),
      ];
      const groups = groupAlerts([alerts]);

      const organizer = new AlertOrganizer(alerts, groups, hideAll);

      expect(organizer.ungroupedAlerts('builder').length).toBe(0);
      expect(organizer.ungroupedAlerts('step').length).toBe(0);
      expect(organizer.ungroupedAlerts('test').length).toBe(0);
    });
    it("should hide an ungrouped alert when all of it's grandchildren are grouped", () => {
      const alerts = [
        new BuilderAlertBuilder().build(),
        new StepAlertBuilder().build(),
        new TestAlertBuilder().build(),
      ];
      const groups = groupAlerts([[alerts[2]]]);

      const organizer = new AlertOrganizer(alerts, groups, hideAll);

      expect(organizer.ungroupedAlerts('builder').length).toBe(0);
    });
  });
});
