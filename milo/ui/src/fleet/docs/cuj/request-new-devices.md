# CUJ: Request New Devices

This Critical User Journey (CUJ) describes how users request new hardware capacity for their projects. The goal is to ensure users get the resources they need while avoiding unnecessary spend by leveraging existing idle capacity.

* **User:** Resource Requester
* **Goal:** I want to ensure my team has enough hardware capacity for testing, so that we can release our product on time.

## The Journey Steps

| Step | Task | Description / How it's served today |
| :--- | :--- | :--- |
| **S1** | **Verify capacity** | The user checks if there is existing idle capacity that can satisfy the demand before requesting new hardware. Served by the **Product Catalog** and **Resource Request Insights** page. |
| **S2** | **File request** | The user submits a structured request. Served by launching a request portal from the UI, selecting from the **Product Catalog**, and auto-validating parameters. |
| **S3** | **Track request** | The user checks the delivery pipeline status without needing to ping engineers manually. Served by tracking cards showing stages (Approved, Staged, Deployed). |

## Friction Points to Avoid
* Users entering the request loop blindly without knowing if their team already owns idle hardware.
* Submitting incomplete requests that lead to long approval loops due to missing details.
* Relying on unmaintained spreadsheets to guess when hardware will land on the lab floor.
