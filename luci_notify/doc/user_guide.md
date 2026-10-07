# LUCI Notify User Guide

LUCI Notify is a service that monitors LUCI builds and sends notifications (primarily emails) when certain conditions are met. This guide explains how to configure LUCI Notify and avoid common pitfalls.

## Core Concepts

LUCI Notify configuration is typically defined in Starlark using [`lucicfg`](https://chromium.googlesource.com/infra/luci/luci-go/+/HEAD/lucicfg/doc/README.md), which generates the project-level `luci-notify.cfg` file.

-   **[`luci.notifier(...)`](https://chromium.googlesource.com/infra/luci/luci-go/+/HEAD/lucicfg/doc/README.md#luci.notifier)**: A Starlark rule defining *when* a notification should be sent (trigger conditions and step filters) and *who* should receive it (recipients). See also the [Starlark rule implementation](https://source.chromium.org/chromium/infra/infra_superproject/+/main:infra/go/src/go.chromium.org/luci/lucicfg/starlark/stdlib/internal/luci/rules/notifier.star).
-   **Connecting to Builders**: A notifier observes one or more Buildbucket builders. You can associate builders with a notifier either by specifying `notified_by = ["<bucket>/<builder>"]` on `luci.notifier(...)`, or by specifying `notifies = ["<notifier-name>"]` on `luci.builder(...)`.
-   **Project Helpers**: Some repositories provide project-specific Starlark wrappers around `luci.notifier` (for example, see Chromium's [`notifiers.star`](https://source.chromium.org/chromium/chromium/src/+/main:infra/config/notifiers.star)).

## Triggering Logic (The "When")

Each `luci.notifier(...)` rule specifies trigger conditions using `on_occurrence` or `on_new_status`.

### `on_occurrence`
Triggers for **every build** that finishes with a status matching one of the specified values.
-   **Use case**: "I want an email for every failure."
-   **Example**:
    ```python
    on_occurrence = ["FAILURE", "INFRA_FAILURE"],
    ```

### `on_new_status`
Triggers only when the build status **changes** to one of the specified values from a different previous status.
-   **Use case**: "I want to know when the build breaks or when it is fixed, but don't spam me for consecutive failures."
-   **Pitfall**: This will not trigger for the very first build of a new builder because there is no previous status to compare against.
-   **Example**:
    ```python
    on_new_status = ["FAILURE", "SUCCESS"],
    ```

### Available Statuses
-   `"SUCCESS"`: Build completed successfully.
-   `"FAILURE"`: Build failed (usually due to test failures or compile errors).
-   `"INFRA_FAILURE"`: Build failed due to infrastructure issues (e.g., bot lost, timeout during setup).
-   `"CANCELED"`: Build was canceled.

---

## Filters

You can further restrict notifications based on which steps failed. Note that in `lucicfg`, step filters cannot be combined with `on_new_status`.

### `failed_step_regexp`
Only notify if at least one failing step matches this regular expression (can be a string or a list of strings).

### `failed_step_regexp_exclude`
Do not notify if any failing step matches this regular expression (often used in combination with `failed_step_regexp`).

> [!IMPORTANT]
> **The "No Steps" Pitfall**: If a build fails before any steps are executed (e.g., an early `INFRA_FAILURE` or a timeout during the "bot_update" phase that isn't reported as a regular step), and you have configured a `failed_step_regexp`, the notification **will not trigger**. This is because LUCI Notify cannot find a matching failing step if no steps ran or if the failure wasn't captured in the steps list.
> 
> If you want to be notified of *all* failures including early infra failures, consider using a separate notification rule without step filters.

---

## Recipients (The "Who")

### Direct Recipients (`notify_emails`)
A list of email addresses to notify.
```python
notify_emails = ["team-alerts@example.com"],
```

### On-call Rotations (`notify_rotation_urls`)
LUCI Notify can fetch the current active on-caller from a JSON rotation URL (e.g., from rotation-proxy). The URL must return a JSON object with an `emails` field containing a list of email address strings.
```python
notify_rotation_urls = ["https://rota-ng.appspot.com/legacy/sheriff.json"],
```

### Blamelist (`notify_blamelist`)
Notifies all users who contributed commits to the build (requires the builder to have an associated repository via `repo` in `luci.builder(...)`).
```python
notify_blamelist = True,
```
You can also restrict the blamelist calculation to specific repositories using `blamelist_repos_whitelist`:
```python
notify_blamelist = True,
blamelist_repos_whitelist = [
    "https://chromium.googlesource.com/chromium/src",
],
```

---

## Examples

### 1. Alert on Every Failure
Simple configuration to get an email for every failed or infra-failed build.
```python
luci.notifier(
    name = "failure-alerts",
    on_occurrence = ["FAILURE", "INFRA_FAILURE"],
    notify_emails = ["developer@example.com"],
    notified_by = ["ci/linux-rel"],
)
```

### 2. Broken/Fixed Alerts
Only notify when the status changes.
```python
luci.notifier(
    name = "status-change-alerts",
    on_new_status = ["FAILURE", "SUCCESS"],
    notify_emails = ["team-leads@example.com"],
    notified_by = ["ci/linux-rel"],
)
```

### 3. Specific Test Suite Failure
Notify a specific team only when their tests fail.
```python
luci.notifier(
    name = "network-team-alerts",
    on_occurrence = ["FAILURE"],
    failed_step_regexp = "network_tests.*",
    notify_emails = ["network-oncall@example.com"],
    notified_by = ["ci/linux-rel"],
)
```

---

## Troubleshooting FAQ

### Why didn't I get an email?
1.  **Check Trigger Conditions**: If using `on_new_status`, did the status actually change from the previous build?
2.  **Step Regexp**: Did the build fail early? If no steps failed matching your regexp (even if the build status is `FAILURE`), no email is sent.
3.  **Allowed Domains**: For security, LUCI Notify only sends emails to the following domains:
    - `@chromium.org`
    - `@google.com`
    - `@grotations.appspotmail.com`
    - `@rotations.google.com`
4.  **Google Groups Configuration**: If the recipient is a Google Group, ensure it is configured to **accept messages from "external" or non-Google addresses** (in Google Groups settings, this setting may be called **"Who can post"** and should be set to **"Anyone on the web"**). Emails are sent from the LUCI Notify service account (typically ending in `@appspot.gserviceaccount.com`), which may be blocked by default group security settings.
5.  **Deduplication**: LUCI Notify deduplicates emails for the same build, recipient, and template.
6.  **Service Logs**: Search for your build ID in the [LUCI Notify logs](https://console.cloud.google.com/logs/query;query=resource.type%3D%22gae_app%22%0Aresource.labels.module_id%3D%22default%22?project=luci-notify). Logs will often explicitly state why a notification was skipped.
    - If you do not have permission to view these logs, please [file a bug for help](https://issuetracker.google.com/issues/new?component=1089151&template=0).
