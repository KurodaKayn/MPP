# Shared Contracts

`openapi.yaml` is the source of truth for cross-runtime API boundary shapes.

Regenerate derived types after editing the contract:

```sh
sh contracts/generate.sh
```

Generated outputs:

- `frontend/src/lib/dashboard/api/generated.ts`
- `backend/internal/contracts/openapi.gen.go`
- `ai-service/contract_schemas.py`
