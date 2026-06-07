# Deferred Editor Media Upload Plan

## Status

The selected direction is to defer object storage uploads until the user clicks the editor save action.

This plan replaces the earlier immediate-upload editor flow. The existing R2 media asset backend remains useful, but the frontend should stop uploading as soon as a file is selected.

## Background

The current editor media flow uploads images to object storage immediately after the user selects, drops, or pastes an image. This makes the editor wait on R2 before the image appears and can create unused R2 objects when the user deletes or replaces an image before saving.

The desired editor behavior is:

- Selecting an image should immediately show a local preview.
- Large images should use a generated local thumbnail preview.
- Small images can preview the original local object URL.
- No R2 upload should happen during selection, paste, drop, delete, or replacement.
- Clicking save should upload only the images still present in the current editor content.
- The save action should wait for image uploads to finish before reporting success.

## Goals

- Improve perceived editor responsiveness by rendering selected images immediately.
- Avoid object storage waste for images the user deletes before saving.
- Preserve stable persisted content by saving `mpp://media/{asset_id}` refs, not local blob URLs or signed URLs.
- Keep publish and prepublish flows using ready object storage media assets.
- Reuse the existing backend media upload, complete, and resolve APIs.

## Non Goals

- No background upload before the save action.
- No persistent browser queue across page reloads in the first implementation.
- No multipart upload support.
- No video upload support.
- No automatic orphan cleanup in this phase.
- No backend worker that uploads browser-selected files without a frontend upload request.

## Current Behavior

The editor currently:

1. Receives image files from file picker, paste, or drop.
2. Calls the media upload API immediately.
3. Uploads the file to R2 with the returned signed PUT URL.
4. Completes the media asset.
5. Resolves a signed GET URL.
6. Inserts the image into TipTap after the upload finishes.

This means the user does not see the image until the upload chain completes.

## Target Behavior

The editor should:

1. Receive image files from file picker, paste, or drop.
2. Create a local media ID for each image.
3. Keep the original `File` in an in-memory upload registry.
4. Generate a local preview URL.
5. Insert the image into TipTap immediately.
6. Mark the image node as a pending local media item.
7. Wait until the user clicks save.
8. Upload only current pending local images.
9. Replace local refs with stable `mpp://media/{asset_id}` refs.
10. Save the project content only after all required uploads complete.

## Data Model In The Editor

Pending local image nodes should carry local-only attributes:

```html
<img
  src="blob:http://localhost:3000/local-preview"
  data-mpp-local-media-id="local-uuid"
  data-mpp-upload-status="pending"
  alt="image.png"
>
```

After save uploads complete, the node should become:

```html
<img
  src="mpp://media/asset-uuid"
  data-mpp-media-id="asset-uuid"
  alt="image.png"
>
```

Runtime preview hydration may replace `src` with a short-lived signed GET URL, but serialization must always write the stable object ref.

## Preview Strategy

Use local previews before upload:

- For small images, create an object URL from the original `File`.
- For large images, generate a local thumbnail preview and keep the original `File` for upload.
- Revoke object URLs when images are removed, replaced, or when the editor unmounts.

Suggested first-pass thresholds:

- Small image: file size <= 1 MB and decoded dimensions <= 2000 px on both axes.
- Large image preview: resize longest side to around 1600 px.
- Upload max size: continue using the existing backend max size until a separate size policy change is made.

## Save Flow

When the user clicks save:

1. Disable save, prepublish, and publish actions.
2. Scan the current editor HTML for `data-mpp-local-media-id`.
3. Match each local ID to the in-memory upload registry.
4. Ignore local files that are no longer present in the current editor content.
5. Upload each remaining file using the existing API sequence:
   - `createProjectMediaUpload`
   - signed URL `PUT`
   - `completeMediaUpload`
   - optional `resolveMediaAssets` for runtime preview refresh
6. Replace each local image node with the completed media asset ref.
7. Serialize the content so persisted HTML contains only stable refs.
8. Call the existing save project content API.
9. Re-enable actions after save finishes or fails.

The save action should report success only after uploads and content persistence both succeed.

## Failure Handling

If any image upload fails:

- Do not call the save project content API.
- Keep the local image preview in the editor.
- Mark the affected local image as failed if the UI supports it.
- Show a toast with the upload error.
- Allow the user to retry by clicking save again.
- If the user deletes the failed image, the next save should not upload it.

If a local image file is missing from the registry:

- Treat it as a save-blocking error.
- Ask the user to reselect the image.
- Do not persist HTML containing `blob:` URLs or local-only media IDs.

## Prepublish And Publish Guard

Prepublish and publish actions must not proceed while the editor contains local pending media.

The first implementation can guard this at the content workspace level:

- If unsaved local media exists, disable prepublish and publish buttons.
- If a user still triggers the action, show a message asking them to save changes first.

This ensures publish services only receive stable `mpp://media/{asset_id}` refs or already hydrated signed URLs generated by backend logic.

## Frontend Implementation Areas

### Editor Media Utilities

Update `frontend/src/components/dashboard/content/editor/content-editor-media.ts` to support:

- Local media ID generation helpers.
- Collection of pending local media IDs.
- Serialization that rejects or preserves pending local media only for runtime use.
- Replacement of local media nodes with completed `mpp://media/{asset_id}` refs.
- Detection of whether content contains pending local media.

### TipTap Image Extension

Update `content-editor-extensions.ts` so image nodes can parse and render:

- `data-mpp-media-id`
- `data-mpp-local-media-id`
- `data-mpp-upload-status`

These attributes should remain internal to editor runtime and should not be persisted after successful save.

### Editor Hook

Update `use-content-tiptap-editor.ts` to:

- Insert local previews immediately.
- Store original files in an in-memory registry.
- Generate thumbnails for large images.
- Expose a save-preparation method that uploads pending images and returns stable serialized content.
- Track whether pending local media exists.
- Clean up object URLs when no longer needed.

### Content Save Controller

Update the dashboard content save flow to:

- Call the editor save-preparation method before saving project content.
- Show upload progress in the save state.
- Disable conflicting actions while upload/save is running.
- Save only after pending local images are converted to stable media refs.

## Backend Implementation Areas

No backend API change is required for the first implementation.

The existing endpoints remain the upload contract:

- `POST /api/user/dashboard/projects/:id/media/uploads`
- `POST /api/user/dashboard/media/:id/complete`
- `POST /api/user/dashboard/media/resolve`

Future backend work may add orphan cleanup or media reference reconciliation, but that is outside this plan.

## Testing Plan

### Frontend Unit Tests

- Selecting an image inserts a local preview without calling upload APIs.
- Large images use a generated thumbnail preview while preserving the original file for upload.
- Removing an unsaved local image prevents it from being uploaded on save.
- Saving uploads only pending images still present in the editor.
- Saving replaces local image refs with `mpp://media/{asset_id}` refs before persistence.
- Upload failure keeps local preview and prevents content save.
- Persisted HTML never contains `blob:` URLs or `data-mpp-local-media-id`.

### Frontend Integration Tests

- Paste/drop/file picker paths all use deferred upload.
- Save button displays upload progress and waits for upload completion.
- Prepublish and publish are blocked while local pending media exists.
- Existing saved `mpp://media/{asset_id}` images still hydrate to signed preview URLs.

### Backend Tests

No new backend tests are required for the first implementation unless the upload API contract changes.

## Rollout Plan

1. Add editor media utilities and tests for local media refs.
2. Extend the TipTap image attributes for local pending images.
3. Change image insertion to local preview only.
4. Add save-time upload orchestration.
5. Connect upload progress and action disabling in the content workspace.
6. Add tests for save blocking, retry, and serialization.
7. Manually test with R2 configured:
   - Select image and verify no R2 object is created before save.
   - Delete selected image before save and verify no upload happens.
   - Save content and verify only current images are uploaded.
   - Refresh editor and verify saved images hydrate from R2.
   - Prepublish and publish after save.

## Risks

### Browser Memory

Large original files remain in memory until save, removal, or page unload. The first implementation should keep the existing max file size and revoke object URLs aggressively.

### Page Reload Before Save

Because files are stored only in memory, refreshing the page before save loses pending local images. This is acceptable for the first implementation because the content was not saved yet.

### Collaboration

Local files cannot be shared through collaborative document state. Pending local images should remain a local editing concern until save converts them into stable media refs.

### Save Latency

Saving may take longer because uploads happen during save. The UI must clearly show upload progress and avoid reporting success until upload and content persistence both finish.

## Open Follow Ups

- Add orphan cleanup for ready media assets no longer referenced by any project content or publication.
- Add a durable upload queue if offline editing or page reload recovery becomes necessary.
- Consider a backend-side image derivative service for thumbnails if local thumbnail quality or performance is insufficient.
