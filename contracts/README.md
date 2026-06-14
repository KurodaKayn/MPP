# Shared Contracts

`openapi.yaml` is the main API contract entrypoint. Reusable schema facts live in
`components/`, path items live in `paths/`, and service-specific files under
`views/` are ref-only codegen entrypoints that must not duplicate schema
definitions.

Regenerate derived types after editing the contract:

```sh
sh contracts/generate.sh
```

Generated outputs:

- `frontend/src/lib/dashboard/api/generated.ts`
- `backend/internal/contracts/openapi.gen.go`
- `browser-worker/internal/contracts/openapi.gen.go`
- `ai-service/contract_schemas.py`

CI runs `script/ci/contracts.sh` to regenerate these files and fail when any
generated output is stale.

Current component/view layout:

- `components/content.yaml` defines shared content and draft schemas.
- `components/project.yaml` defines project, publication, and publish-result schemas.
- `components/workspace.yaml` defines workspace membership, invite, and activity schemas.
- `components/collab.yaml` defines collaborative document schemas.
- `components/account.yaml` defines account connection and browser session schemas.
- `components/ai.yaml` defines AI editing request and response schemas.
- `components/template.yaml` defines reusable content template schemas.
- `components/brand.yaml` defines brand profile schemas.
- `components/browser-worker.yaml` defines the internal browser-worker request and
  response models.
- `paths/browser-worker.yaml` defines the internal browser-worker path items.
- `views/backend.openapi.yaml` selects the schema type surface used by
  `backend`.
- `views/frontend.openapi.yaml` selects only the public schema surface used by
  `frontend`.
- `views/ai-service.openapi.yaml` selects the small content schema subset used by
  `ai-service`.
- `views/browser-worker.openapi.yaml` selects only the browser-worker schema
  types used by `browser-worker`.
