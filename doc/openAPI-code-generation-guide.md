# OpenAPI Code Generation Guide

This project uses a contract-first architecture based on a shared OpenAPI specification. All API definitions and data models are maintained in `/contracts` and used to generate strongly typed code for each service.

## Contract Structure

```text
contracts/
├── openapi.yaml      # Root OpenAPI specification
├── components/       # Shared schemas and models
├── paths/            # API endpoint definitions
├── views/            # Service-specific API views
└── generate.sh       # Code generation entrypoint
```

## Architecture

- `components` contains shared schema definitions.
- `paths` defines API endpoints and references schemas.
- `views` expose only the APIs required by a specific service.
- Generated code is derived from service views rather than the full specification.

## Generated Artifacts

| Service        | Generated Output                                 |
| -------------- | ------------------------------------------------ |
| Backend        | backend/internal/contracts/openapi.gen.go        |
| Frontend       | frontend/src/lib/dashboard/api/generated.ts      |
| AI Service     | ai-service/contract_schemas.py                   |
| Browser Worker | browser-worker/internal/contracts/openapi.gen.go |

## Workflow

1. Add or update schemas in `components`.
2. Define or update endpoints in `paths`.
3. Reference required endpoints in the appropriate `views`.
4. Regenerate code:

```bash
sh contracts/generate.sh
```

All generated files are updated automatically and should not be edited manually.