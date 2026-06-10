# MPP: multi-platform-poster

<p align="center">
  <strong>English</strong> | <a href="README.zh-CN.md">Chinese</a>
</p>

<p align="center">
  <img src="doc/assets/mpp-with-name-white.svg" alt="MPP logo" width="640" />
  <br>
  <img src="https://img.shields.io/badge/License-AGPL--3.0-blue" alt="License" />
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Next.js-App_Shell-000000?style=for-the-badge&logo=nextdotjs&logoColor=white" alt="Next.js" />
  <img src="https://img.shields.io/badge/React-Component-20232A?style=for-the-badge&logo=react&logoColor=61DAFB" alt="React" />
  <img src="https://img.shields.io/badge/TypeScript-Frontend-3178C6?style=for-the-badge&logo=typescript&logoColor=3178C6" alt="TypeScript" />
  <img src="https://img.shields.io/badge/Tailwind_CSS-UI_Styling-06B6D4?style=for-the-badge&logo=tailwindcss&logoColor=06B6D4" alt="Tailwind CSS" />
  <br>
  <img src="https://img.shields.io/badge/shadcn-UI_Primitives-000000?style=for-the-badge&logo=shadcnui&logoColor=white&labelColor=000000" alt="shadcn" />
  <img src="https://img.shields.io/badge/WXT-Extension_Shell-67D8EF?style=for-the-badge&logo=wxt&logoColor=66FF66" alt="WXT" />
  <img src="https://img.shields.io/badge/Go-Backend_Runtime-00ADD8?style=for-the-badge&logo=go&logoColor=00ADD8" alt="Go" />
  <img src="doc/assets/badges/python.svg" alt="Python" />
  <br>
  <img src="https://img.shields.io/badge/FastAPI-AI_HTTP_API-05998B?style=for-the-badge&logo=fastapi&logoColor=05998B" alt="FastAPI" />
  <img src="https://img.shields.io/badge/LangChain-LLM_Boundary-1C3C3C?style=for-the-badge&logo=langchaincorporate&logoColor=1C3C3C" alt="LangChain" />
  <img src="https://img.shields.io/badge/PostgreSQL-Durable_State-4169E1?style=for-the-badge&logo=postgresql&logoColor=4169E1" alt="PostgreSQL" />
  <br>
  <img src="doc/assets/badges/echo.svg" alt="Echo" />
  <img src="doc/assets/badges/gorm.svg" alt="GORM" />
  <img src="doc/assets/badges/chromium.svg" alt="Chromium" />
  <br>
  <img src="https://img.shields.io/badge/Docker-Local_Stack-2496ED?style=for-the-badge&logo=docker&logoColor=2496ED" alt="Docker" />
  <img src="https://img.shields.io/badge/Traefik-Edge_Proxy-24A1C1?style=for-the-badge&logo=traefikproxy&logoColor=24A1C1" alt="Traefik" />
  <img src="doc/assets/badges/redis.svg" alt="Redis" />
</p>

## Overview

MPP is a multi-platform content publishing system for creators and operations teams. It helps manage content projects, platform-specific adaptation, publishing status tracking, and AI-assisted processing from a unified workspace.

## Demo Video(Click to watch)

<p align="center">
  <a href="https://youtu.be/_Dh35Ksb9RQ">
    <img src="https://img.youtube.com/vi/_Dh35Ksb9RQ/maxresdefault.jpg" alt="Watch the MPP demo on YouTube" width="100%" />
  </a>
</p>

## Project Highlights

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

## For Developer

### Quick Start

Choose the setup path that matches how you want to run MPP:

- [Production setup](doc/setup.md)
- [Docker development setup](doc/setup-dev.md)
- [Standalone local development setup](doc/setup-standalone.md)

### Architecture

![MPP Overall Architecture](./doc/assets/module-architecture/project-overall-architecture.png)

### Moudle Design

For detailed design notes for each module, see [module-design.md](doc/module-design.md).
For a module-by-module technology stack breakdown, see [tech-stack.md](doc/tech-stack.md).
For module external interfaces and runtime parameters, see [module-interface.md](doc/module-interface.md)
