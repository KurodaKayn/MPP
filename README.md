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

## Demo Videos

- [Bilibili demo](https://www.bilibili.com/video/BV1wQVD6mEA6)
- [YouTube demo](https://youtu.be/_Dh35Ksb9RQ)

## Project Innovations

MPP focuses on turning multi-platform publishing into a structured, automatable workflow. Each innovation introduces a specific technical layer to solve a concrete publishing problem.

### 1. Platform draft adaptation

MPP introduces a pre-publish adapter pipeline with versioned JSON draft contracts. This solves the format mismatch between platforms: WeChat can receive HTML, Zhihu can receive Markdown, X can receive length-limited text, and future platforms can add their own draft formats without changing the editor core.

### 2. Adapter-based platform isolation

MPP uses Go publisher and platform adapter interfaces to isolate third-party rules. This solves the problem of different account models, validation rules, API styles, and publishing flows being mixed into the main business logic.

### 3. Remote browser login

MPP introduces `browser-worker`, disposable Chromium sessions, CDP cookie capture, Redis session tokens, and PostgreSQL audit records. This solves cookie-based or QR-code login for platforms such as Douyin and Zhihu without exposing raw cookies, CDP endpoints, or browser state to the frontend.

### 4. Reviewable AI editing

MPP uses a separate FastAPI AI service with streaming responses and a proposal-confirmation workflow. This solves the risk of AI directly overwriting official content: generated edits are previewed, compared, and accepted by the user before they update drafts.

### 5. VM-style publishing without plugins

For platforms without stable public publishing APIs, MPP uses a backend-controlled virtualized browser runtime instead of browser plugins. This solves Zhihu- and Douyin-style publishing by keeping draft filling, media upload, and publish actions in an auditable server-side flow rather than scattering automation scripts across user browsers.

## Architecture

![MPP Overall Architecture](doc/assests/project-overall-architecture.png)

For detailed design notes for each module, see [module-design.md](doc/module-design.md).
For a module-by-module technology stack breakdown, see [tech-stack.md](doc/tech-stack.md).

## How to Quick Start

See [Setup Guide](doc/setup.md)
