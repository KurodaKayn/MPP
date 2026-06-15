# Node Dependency Management

MPP uses one root pnpm workspace for all Node packages:

- `frontend`
- `extension`
- `collab-service`

The root `pnpm-workspace.yaml` is the source of truth for shared dependency
versions. Use the default catalog for shared React, Yjs, Tailwind, TypeScript,
Vitest, oxlint, oxfmt, and related tooling. Use the `tiptap` catalog for all
Tiptap packages so editor and collaboration upgrades happen as one transaction.

## Install

Run installs from the repository root and filter to the package you are working
on:

```bash
pnpm install --filter frontend...
pnpm install --filter mpp-extension-publisher...
pnpm install --filter collab-service...
```

## Update Policy

- Change shared versions in root `pnpm-workspace.yaml`, not in package-local
  manifests.
- Regenerate only the root lockfile with `pnpm install --lockfile-only`.
- Keep package-local `package.json` files focused on package ownership: they
  declare which dependencies a package uses, while the root catalog declares
  shared versions.
- Check available updates with `pnpm deps:outdated`.
