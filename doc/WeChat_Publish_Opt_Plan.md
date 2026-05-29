# WeChat Publish Optimization Plan

This document outlines identified issues and the optimization roadmap for the WeChat Official Account publishing module. These items are prioritized based on their impact on system reliability and data integrity.

## 1. High Priority (P1)

### 1.1 Incorrect Success Logging on API Failure
*   **Issue:** The `Client.Publish` method parses `errcode` and `errmsg` but currently returns `nil` (no error) even when the code is non-zero (except for the specifically handled `48001`). This leads to failed API calls (due to invalid media, rate limits, or expired tokens) being incorrectly marked as `status=published` with a generated placeholder URL.
*   **Optimization:** Modify the `Publish` method to return an explicit error for any non-zero `errcode`. Ensure the calling service treats every unhandled non-zero code as a publication failure.

### 1.2 Placeholder JSON in Draft Content
*   **Issue:** The `AdaptContent` method currently returns a placeholder JSON `{"status": "ready_to_process"}` instead of the actual adapted HTML content. Since the `Publish` method uses `pub.AdaptedContent` directly, the resulting WeChat drafts contain the placeholder JSON string rather than the intended article body.
*   **Optimization:** Implement the actual adaptation logic to sync `Project.SourceContent` into the `AdaptedContent` field before publishing, ensuring the HTML is correctly structured for the WeChat API.

## 2. Medium Priority (P2)

### 2.1 Unhandled Database Update Failures
*   **Issue:** The `PublishProject` service calls `s.db.Model(&pub).Updates(updates)` but does not verify the `.Error` result. If the database update fails (e.g., due to connection loss), the API still reports success to the user, causing a discrepancy between the platform's state and the local record.
*   **Optimization:** Add explicit error checking for all GORM update operations. Implement a rollback mechanism or return an internal error if the local state cannot be synchronized with the remote publishing result.

### 2.2 Incomplete Image Compression Loop
*   **Issue:** The image compressor currently performs quality reduction once (fixed at 50%) without re-validating the resulting file size. Extremely large images may still exceed WeChat's 2MB limit after a single compression pass, leading to silent upload failures.
*   **Optimization:** Implement a recursive or iterative compression loop. The processor should continue to downscale resolution or decrease quality until the file size is guaranteed to be below the platform limit or until a hard failure threshold is reached.

## 3. Low Priority (P3)

### 3.1 Code Style & Formatting Consistency
*   **Issue:** Several modified Go files deviate from the official `gofmt` standards. This can cause CI build failures in environments that enforce strict linting and introduces "noise" during code reviews.
*   **Optimization:** Run `gofmt -w` across all modified directories. Integrate a pre-commit hook or linting step into the local development workflow to ensure all files are automatically formatted before submission.
