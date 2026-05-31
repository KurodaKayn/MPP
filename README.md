# MPP: multi-platform-poster

<p align="center">
  <img src="doc/assests/mpp-with-name-white.svg" alt="MPP logo" width="360" />
  <br>
  <img src="https://img.shields.io/badge/License-AGPL--3.0-blue" alt="License" />
</p>

![Next.js](https://img.shields.io/badge/Next.js-16-000000?style=for-the-badge&logo=nextdotjs&logoColor=white)
![React](https://img.shields.io/badge/React_19-20232A?style=for-the-badge&logo=react&logoColor=61DAFB)
![TypeScript](https://img.shields.io/badge/TypeScript-5-3178C6?style=for-the-badge&logo=typescript&logoColor=white)
![Tailwind CSS](https://img.shields.io/badge/Tailwind_CSS-4-06B6D4?style=for-the-badge&logo=tailwindcss&logoColor=white)
![shadcn](https://img.shields.io/badge/shadcn-4-000000?style=for-the-badge&logo=shadcnui&logoColor=white)
![Go](https://img.shields.io/badge/Go-1.26-00ADD8?style=for-the-badge&logo=go&logoColor=white)
![Python](https://img.shields.io/badge/Python-3.12-3776AB?style=for-the-badge&logo=python&logoColor=white)
![FastAPI](https://img.shields.io/badge/FastAPI-0.136-05998B?style=for-the-badge&logo=fastapi&logoColor=white)
![LangChain](https://img.shields.io/badge/LangChain-1.3-1C3C3C?style=for-the-badge&logo=langchaincorporate&logoColor=white)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-17-4169E1?style=for-the-badge&logo=postgresql&logoColor=white)
![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?style=for-the-badge&logo=docker&logoColor=white)

## Overview

MPP is a multi-platform content publishing system for creators and operations teams. It helps manage content projects, platform-specific adaptation, publishing status tracking, and AI-assisted processing from a unified workspace.

## Project Innovations

MPP focuses its innovation on platform-specific draft adaptation, third-party platform isolation, remote login, reviewable AI editing, and VM-style publishing execution without requiring browser plugins.

### 1. Explicit pre-publish adaptation pipeline

MPP separates source draft saving from platform draft generation. Users maintain one canonical source draft, then sync it into platform-specific derived drafts during the pre-publish stage:

- WeChat Official Accounts use HTML drafts.
- Zhihu uses Markdown drafts.
- X uses length-limited plain text.
- Future platforms can extend this model to image posts, video feeds, note-style posts, and other content formats.

Each derived draft can record `schema_version`, `format`, `source_revision`, `generated_by`, and asset metadata, making the adaptation process easier to preview, debug, trace, and extend with future AI agent intervention.

### 2. Platform adapters isolate third-party differences

Platform-specific rules are contained behind publisher and adapter boundaries. The backend workflow only needs to coordinate projects, platform targets, state transitions, and publishing jobs. When adding a new platform, the system can implement configuration validation, content adaptation, account connection, and publishing behavior around a stable platform key without changing the editor core or the general publishing flow.

This allows API-based platforms and browser-based platforms to coexist. WeChat Official Accounts and X can use official APIs or OAuth, while platforms such as Douyin and Zhihu that depend on web login state can reuse the remote browser capability.

### 3. Isolated remote browser login

For platforms that require cookie-based login or QR-code login, MPP uses a dedicated `browser-worker`. It starts a disposable Chromium runtime per session, streams the controlled browser to the user for login, and then lets the backend capture the required cookies and account information through CDP.

The frontend never receives raw cookies, CDP endpoints, or container details. Redis manages short-lived sessions, locks, stream tokens, and TTLs, while PostgreSQL stores auditable final state. This reduces the need for manual cookie copying while keeping sensitive browser state out of the frontend.

### 4. Reviewable AI streaming edit workflow

AI editing does not directly overwrite content. Instead, it follows a streaming generation, proposal preview, and diff confirmation workflow. The `ai-service` owns prompt construction and model calls, the Go backend handles authentication, context checks, and streaming proxying, and the frontend presents incremental output for user confirmation.

This lets AI assist with source draft polishing, platform draft rewriting, and pre-publish calibration while keeping a clear human review boundary before any generated content becomes official.

### 5. VM-style publishing execution without browser plugins

For platforms such as Zhihu and Douyin, where stable public publishing APIs may be unavailable or where publishing depends heavily on logged-in creator dashboards, MPP does not require users to install browser plugins. Instead, it uses a backend-controlled, VM-style browser runtime to complete login, draft filling, media upload, and publishing actions, turning platform operations into an auditable, queueable, retryable server-side workflow.

Compared with plugin-based automation, this approach gives the system a more consistent permission boundary and execution environment. Users only complete the required authorization inside the controlled remote browser; publishing logic is then executed by `browser-worker` and platform adapters instead of being scattered across local user browsers.

## Architecture

![MPP Overall Architecture](doc/assests/project-overall-architecture.png)

## How to Quick Start

See [Setup Guide](doc/setup.md)
