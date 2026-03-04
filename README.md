<div align="center">

# 🛡️ SecureVault — Backend

**A production-grade Go backend for cloud file management with AI-powered intelligence**

[![Go](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-Neon-4169E1?logo=postgresql&logoColor=white)](https://neon.tech)
[![GraphQL](https://img.shields.io/badge/GraphQL-gqlgen-E10098?logo=graphql&logoColor=white)](https://gqlgen.com)
[![Docker](https://img.shields.io/badge/Docker-Alpine-2496ED?logo=docker&logoColor=white)](https://hub.docker.com)
[![Deployed on HF](https://img.shields.io/badge/HuggingFace-Spaces-FFD21E?logo=huggingface&logoColor=black)](https://cheesechat-securevault-backend.hf.space)

[Live API](https://cheesechat-securevault-backend.hf.space/health) · [Swagger Docs](https://cheesechat-securevault-backend.hf.space/swagger/) · [Frontend](https://secure-vault-storage-pearl.vercel.app) · [Report Bug](../../issues)

</div>

---

## 📋 About

SecureVault Backend is the API server powering a full-featured cloud file management platform. It provides content-addressable file storage with deduplication, AI-powered document analysis, Google OAuth, and a dual REST + GraphQL API layer.

Built with **Go 1.24**, backed by **PostgreSQL (Neon)**, with **AWS S3** for production storage and **Groq/Gemini** for AI features.

---

## ✨ Features

### 📦 Storage Engine
- SHA-256 content-addressable blobs with reference-counted deduplication
- Streaming upload — hash + MIME detect + size validate in one pass
- S3 production storage with hash-sharded local fallback

### 🔐 Auth & Security
- JWT + bcrypt (cost 12) with Google OAuth (server-side token verification)
- Three-way Google login: existing user / email linking / new account
- Per-user rate limiting (token bucket) and storage quota enforcement
- CORS + HSTS + security headers

### 🤖 AI Features
- Provider-agnostic dispatch (Groq / Gemini) with auto-detection
- Auto-tagging — tags, descriptions, folder suggestions per file
- Image analysis via LLaMA 3.2 Vision (base64 payloads)
- Document summarization with map-reduce chunking for large docs (>120K chars)
- Iterative refinement with version history (up to 10 entries)
- Per-user daily rate limits (24h sliding window)

### 📁 File Management
- Upload, download, nested folders, soft-delete with trash/restore
- Public share links for files and folders
- Format conversion (TXT/CSV/MD/HTML → PDF/XLSX) with background jobs

### 🛠️ Admin
- Platform-wide statistics and per-user storage breakdown
- User quota management and account suspension

---

## 🧰 Tech Stack

| | Technology |
|---|---|
| 🔧 Language | Go 1.24 |
| 🌐 APIs | Gorilla Mux (REST), gqlgen (GraphQL), Swagger/OpenAPI |
| 🗄️ Database | PostgreSQL on Neon (serverless) |
| ☁️ Storage | AWS S3 with local-disk fallback |
| 🔐 Auth | JWT, bcrypt, Google OAuth 2.0 |
| 🤖 AI | Groq (compound, LLaMA 3.2 Vision), Google Gemini |
| 🚀 Deploy | Docker on Hugging Face Spaces |

---

## 🏗️ Project Structure

```
src/
├── main.go                    # Entry point, graceful shutdown
├── api/                       # 🌐 REST handlers
│   ├── auth_handlers.go       #   Signup, login, Google OAuth
│   ├── files_handlers.go      #   File CRUD, upload, download, AI triggers
│   ├── folders_handlers.go    #   Folder CRUD, sharing
│   ├── summary_handlers.go    #   AI summary + refinement
│   ├── conversion_handlers.go #   File format conversion
│   ├── admin_handlers.go      #   Admin operations
│   ├── stats_handlers.go      #   User statistics
│   ├── public_*.go            #   Unauthenticated access
│   └── middleware/limits.go   #   Rate limiter + quota
├── services/                  # ⚙️ Business logic
│   ├── auth_service.go        #   JWT, bcrypt, Google OAuth
│   ├── storage_service.go     #   S3/local blobs, dedup, MIME validation
│   ├── file_service.go        #   File metadata, soft-delete
│   ├── folder_service.go      #   Folder hierarchy, cascades
│   ├── ai_tag_service.go      #   Groq/Gemini tagging dispatch
│   ├── ai_summary_service.go  #   Summarization, chunking, refinement
│   ├── conversion_service.go  #   Format conversion orchestration
│   └── converters.go          #   Converters (PDF, XLSX)
├── models/                    # 📦 Data models (User, File, Blob, Folder…)
├── graphql/                   # 📡 GraphQL schema, resolvers, codegen
├── internal/
│   ├── app/app.go             #   Bootstrap, DI, routing, middleware
│   └── db/db.go               #   Connection pool, migration runner
├── migrations/                # 🗄️ 14 ordered SQL migrations (001_ → 014_)
└── swaggerdocs/               # 📄 Auto-generated OpenAPI specs
```

---

## 🔌 API Overview

All REST routes are under `/api/v1/`. Full interactive docs at [`/swagger/`](https://cheesechat-securevault-backend.hf.space/swagger/).

| Area | Endpoints | Key Operations |
|---|---|---|
| 🔐 **Auth** | `/auth/signup`, `/auth/login`, `/auth/google` | Register, login, Google OAuth |
| 📁 **Files** | `/files`, `/files/{id}`, `/files/{id}/download` | Upload, list, download, delete, move |
| 🤖 **AI Tags** | `/files/{id}/ai-tags`, `/files/{id}/ai-describe` | Auto-tag, describe, bulk tag |
| 📝 **AI Summary** | `/files/{id}/ai-summary`, `.../refine` | Summarize, refine, get status |
| 📂 **Folders** | `/folders`, `/folders/{id}/share` | CRUD, share links |
| 🔄 **Conversions** | `/files/{id}/convert`, `/conversions/{jobId}` | Convert format, poll, download |
| 🌍 **Public** | `/public/files/{id}`, `/p/{token}` | Unauthenticated file/folder access |
| 🛠️ **Admin** | `/admin/stats`, `/admin/files`, `/admin/users/{id}/quota` | Stats, oversight, quota mgmt |
| 📡 **GraphQL** | `/graphql`, `/graphql/playground` | Flexible queries & mutations |

---

## 🚀 Getting Started

### Prerequisites

- **Go** ≥ 1.24
- **PostgreSQL** 15+ (or a [Neon](https://neon.tech) database)

### 1. Clone & Install

```bash
git clone https://github.com/<your-username>/securevault-backend.git
cd securevault-backend
go mod download
```

### 2. Configure Environment

```bash
cp .env.example .env
```

Edit `.env` with your values:

```env
DB_URL=postgresql://user:pass@host/dbname?sslmode=require
JWT_SECRET=your-strong-random-secret
PORT=8080
ENVIRONMENT=development
AI_PROVIDER=groq
GROQ_API_KEY=gsk_your_key          # free at console.groq.com
GOOGLE_CLIENT_ID=your-client-id    # optional
STORAGE_PATH=./storage
```

### 3. Run

```bash
go run ./src
```

Migrations run automatically. Server starts on `:8080`.

### 4. Verify

```bash
curl http://localhost:8080/health
# {"status":"ok","service":"securevault-backend"}
```

---

## 🐳 Docker

```bash
docker build -t securevault-backend .
docker run -p 8080:8080 --env-file .env securevault-backend
```

---

## ⚙️ Environment Variables

| Variable | Required | Description |
|---|---|---|
| `DB_URL` | ✅ | PostgreSQL connection string |
| `JWT_SECRET` | ✅ (prod) | JWT signing key |
| `GROQ_API_KEY` | — | Enables AI features |
| `GOOGLE_CLIENT_ID` | — | Enables Google OAuth |
| `S3_BUCKET_NAME` | — | Enables S3 storage (local fallback if unset) |
| `CORS_ALLOWED_ORIGIN` | — | Frontend origin (default: `*`) |
| `PORT` | — | Server port (default: `8080`) |
| `STORAGE_PATH` | — | File storage dir (default: `./storage`) |

Full reference in [`.env.example`](.env.example).

---

## 🏛️ Architecture

```
HTTP Server (Gorilla Mux + Middleware)
 ├── 🌐 REST Handlers ──┐
 ├── 📡 GraphQL ────────┤──▶ Service Layer
 └── 📄 Swagger UI      │     ├── AuthService ────▶ JWT / bcrypt / Google
                        │     ├── StorageService ─▶ S3 / Local Disk
                        │     ├── FileService ────▶ PostgreSQL (Neon)
                        │     ├── AiTagService ───▶ Groq / Gemini APIs
                        │     ├── AiSummaryService ▶ Groq (map-reduce)
                        │     └── ConversionService ▶ Local converters
```

**Key design decisions:**
- **📦 Content-addressable storage** — blobs keyed by SHA-256, deduped via ref counting
- **🤖 Provider-agnostic AI** — `callAI()` dispatches to Groq or Gemini; adding providers is one method
- **🔌 Hybrid API** — GraphQL for queries, REST for binary ops (upload/download)
- **💉 Dependency injection** — all services wired in `NewApp()`, no globals

---

## 📄 License

This project is for educational and portfolio purposes.

---

<div align="center">

Built with 🔧 Go · 🗄️ PostgreSQL · 📡 GraphQL · 🤖 Groq AI · ☁️ AWS S3

</div>
