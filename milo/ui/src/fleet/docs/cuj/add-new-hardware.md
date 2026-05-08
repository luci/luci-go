# CUJ: Add New Hardware to the Fleet

This Critical User Journey (CUJ) describes the process of fulfilling approved resource requests by planning space and deploying new hardware in the lab.

* **User:** Fulfillment Manager
* **Goal:** I want to get new hardware deployed and running as fast as possible, so that requesting teams can start testing.

## The Journey Steps

| Step | Task | Description / How it's served today |
| :--- | :--- | :--- |
| **S1** | **Review requests** | The user audits the queue of pending requests and identifies those waiting for fulfillment or flagged as stalled. Served by the **Resource Request Insights** and related planner pages. |
| **S2** | **Find space** | The user locates vacant physical slots (ports, shelf space) meeting power and spatial requirements. Served by location filters in the **Device List** page. |
| **S3** | **Stage and deploy** | The user groups hostnames and triggers automated fulfillment/provisioning tasks. Handled by triggering tasks from the UI or CLI. |
| **S4** | **Verify success** | The user confirms the hardware is online and linked deployment tickets are resolved. |

## Friction Points to Avoid
* Slipped requests falling out of view due to fragmented tracking surfaces.
* Navigating complex spreadsheets to perform manual space and power capacity math.
* Manual toil in filing downstream deployment tickets one by one.
