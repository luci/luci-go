# Priority Scoring Rules Admin Panel E2E Recipe

## Goal
Verify that FLOPs leads can view, create, edit, and delete priority scoring rules targeting basic device attributes using AIP-160 filters and point weights against a live local backend and database.

## Prerequisites
- Local Postgres running on port `5432` with migrations applied (`make run-db` in `infra/fleetconsole`).
- Local backend running on port `8800` (`make run-local-db` in `infra/fleetconsole`).
- Vite UI dev server running on port `8080` with `VITE_OVERRIDE_FLEET_CONSOLE_HOST=localhost:8800` in `milo/ui`.
- Chrome running with remote debugging on port `9222` (`DISPLAY=:20` or `--headless=new`).

---

## Step 1: Open ChromeOS Repairs Page & Verify Priority Rules Inline Panel

### Action
Navigate to the ChromeOS repairs dashboard and verify the inline Priority Scoring Rules panel.

```json
[
  {"action": "navigate", "url": "http://localhost:8080/ui/fleet/p/chromeos/repairs", "waitUntil": "none"},
  {"action": "sleep", "duration": "3s"},
  {"action": "screenshot", "file": "/tmp/e2e_priority_rules_1_panel_visible.png"}
]
```

### Visual Verification
- [ ] Inspect `/tmp/e2e_priority_rules_1_panel_visible.png`.
- [ ] Verify the inline configuration panel is rendered with title "Priority Scoring Rules" in the left column.
- [ ] Verify active rules from database are rendered (or empty state if no rules exist).
- [ ] Verify `Add rule` button is visible.

---

## Step 2: Add New Draft Rule & Apply

### Action
Click `Add rule`, fill in filter expression (`pool = "DUT_POOL_QUOTA"`) and weight (`150`), then click "Apply".

```json
[
  {"action": "eval", "expression": "(() => { const b = document.querySelector('[data-testid=\"add-priority-rule-button\"]') || Array.from(document.querySelectorAll('button')).find(el => el.textContent.includes('Add rule')); if (b) b.click(); return 'clicked'; })()"},
  {"action": "sleep", "duration": "1s"},
  {"action": "eval", "expression": "(() => { const inputs = document.querySelectorAll('input'); for (const inp of inputs) { if (inp.dataset.testid && inp.dataset.testid.includes('rule-filter-input-draft')) { const set = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set; set.call(inp, 'pool = \"DUT_POOL_QUOTA\"'); inp.dispatchEvent(new Event('input', { bubbles: true })); } if (inp.dataset.testid && inp.dataset.testid.includes('rule-weight-input-draft')) { const set = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set; set.call(inp, '150'); inp.dispatchEvent(new Event('input', { bubbles: true })); } } return 'filled'; })()"},
  {"action": "sleep", "duration": "1s"},
  {"action": "screenshot", "file": "/tmp/e2e_priority_rules_2_draft_filled.png"},
  {"action": "eval", "expression": "(() => { const btn = document.querySelector('button[data-testid*=\"rule-apply-button-draft\"]') || Array.from(document.querySelectorAll('button')).find(el => el.textContent === 'Apply'); if (btn) btn.click(); return 'applied'; })()"},
  {"action": "sleep", "duration": "2s"},
  {"action": "screenshot", "file": "/tmp/e2e_priority_rules_3_persisted.png"}
]
```

### Visual Verification
- [ ] In `/tmp/e2e_priority_rules_2_draft_filled.png`, verify the new draft row appears highlighted with inputs and an "Apply" button.
- [ ] In `/tmp/e2e_priority_rules_3_persisted.png`, verify the new rule is persisted in the list as an active rule with its own delete icon.

---

## Step 3: In-Place Rule Editing (Per-Rule Apply)

### Action
Modify an existing rule's weight (e.g. change to `450`). Notice the per-rule "Apply" button appears only on the edited row.

```json
[
  {"action": "fill", "selector": "[data-testid^='rule-weight-input-'], input[type='number']", "value": "450"},
  {"action": "sleep", "duration": "1s"},
  {"action": "screenshot", "file": "/tmp/e2e_priority_rules_4_edited.png"},
  {"action": "click", "selector": "button[data-testid^='rule-apply-button-'], button:has-text('Apply')"},
  {"action": "sleep", "duration": "2s"},
  {"action": "screenshot", "file": "/tmp/e2e_priority_rules_5_applied.png"}
]
```

### Visual Verification
- [ ] In `/tmp/e2e_priority_rules_4_edited.png`, verify the blue "Apply" button appears specifically on the modified row.
- [ ] In `/tmp/e2e_priority_rules_5_applied.png`, verify the mutation succeeded, the weight shows `450`, and the "Apply" button disappears.

---

## Step 4: Edge Case Verification (Invalid Syntax)

### Action
Try adding an invalid AIP-160 expression (e.g. `invalid && syntax`) and verify the error alert.

```json
[
  {"action": "eval", "expression": "(() => { const b = document.querySelector('[data-testid=\"add-priority-rule-button\"]') || Array.from(document.querySelectorAll('button')).find(el => el.textContent.includes('Add rule')); if (b) b.click(); return 'clicked'; })()"},
  {"action": "sleep", "duration": "1s"},
  {"action": "eval", "expression": "(() => { const inputs = document.querySelectorAll('input'); for (const inp of inputs) { if (inp.dataset.testid && inp.dataset.testid.includes('rule-filter-input-draft')) { const set = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set; set.call(inp, 'invalid && syntax'); inp.dispatchEvent(new Event('input', { bubbles: true })); } if (inp.dataset.testid && inp.dataset.testid.includes('rule-weight-input-draft')) { const set = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set; set.call(inp, '100'); inp.dispatchEvent(new Event('input', { bubbles: true })); } } return 'filled'; })()"},
  {"action": "sleep", "duration": "1s"},
  {"action": "eval", "expression": "(() => { const btn = document.querySelector('button[data-testid*=\"rule-apply-button-draft\"]') || Array.from(document.querySelectorAll('button')).find(el => el.textContent === 'Apply'); if (btn) btn.click(); return 'applied'; })()"},
  {"action": "sleep", "duration": "2s"},
  {"action": "screenshot", "file": "/tmp/e2e_priority_rules_6_invalid_syntax_error.png"}
]
```

### Visual Verification
- [ ] Inspect `/tmp/e2e_priority_rules_6_invalid_syntax_error.png`.
- [ ] Verify red error banner appears with clear validation message (`InvalidArgument`).

