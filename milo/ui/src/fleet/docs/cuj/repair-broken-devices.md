# CUJ: Repair Broken Devices

This Critical User Journey (CUJ) describes the end-to-end process of identifying, prioritizing, and repairing broken devices in the fleet. This is a major maintenance goal that ensures test capacity is restored quickly.

* **User:** Lab Technician
* **Goal:** I want to get broken devices back into the test pool as fast as possible, so that developers can run their tests without delay.

## The Journey Steps

| Step | Task | How it's done today / Serving functionality |
| :--- | :--- | :--- |
| **S1** | **Check priority cases** | • **Android:** Uses the **Repairs** page in Fleet Console.<br>• **ChromeOS:** Uses Chrome Fleet Dashboard or **Admin Tasks** page in Fleet Console.<br>• **Browser:** Direct in Swarming UI. |
| **S2** | **Divide up work** | • **Android:** Split into work queues.<br>• **ChromeOS/Browser:** Cases assigned in **Issue Tracker**. |
| **S3** | **Search for devices** | • **Android:** Search in Mobile Harness or **Fleet Console**.<br>• **ChromeOS:** Search in Swarming or **Fleet Console**. |
| **S4** | **Identify root cause** | • **Android:** Unhealthy reason in Mobile Harness.<br>• **ChromeOS:** Check labels (e.g., servo state) and playbooks.<br>Served by viewing device details in **Fleet Console**. |
| **S5** | **Physically find machine**| Hostname usually contains location info. For ChromeOS, location is looked up in **Fleet Console** or via CLI. |
| **S6** | **Apply fixes** | Physical interaction (cable swap, etc.) or triggering automated recovery actions. |
| **S7** | **Verify success** | • **Android:** Wait for job and check status in **Fleet Console** or Mobile Harness.<br>• **ChromeOS:** Check status in **Fleet Console** or Swarming (`dut_state=READY`). |
| **S8** | **Log actions** | • **Android:** Update Mobile Harness logging.<br>• **ChromeOS:** Update **Issue Tracker** custom fields. |

## Friction Points to Avoid
* Data staleness in priority lists leading to wasted effort.
* Having to cross-reference multiple tools (Swarming, UFS, Lab Management Systems) to find a single device's state.
* Lack of spatial grouping in task lists, causing technicians to waste time walking the lab floor.
