# SecureVault Backend

A Go backend for a cloud file management platform with content-addressable storage, AI-powered document analysis, and dual REST + GraphQL APIs.

**Live:** [cheesechat-securevault-backend.hf.space](https://cheesechat-securevault-backend.hf.space) | **Frontend:** [secure-vault-storage-pearl.vercel.app](https://secure-vault-storage-pearl.vercel.app) | **Docs:** [Swagger UI](https://cheesechat-securevault-backend.hf.space/swagger/)

---

## Tech Stack

| | Technology |
|---|---|
| **Language** | Go 1.24 |
| **APIs** | Gorilla Mux (REST), gqlgen (GraphQL), Swagger/OpenAPI |
| **Database** | PostgreSQL on Neon (serverless) |
| **Storage** | AWS S3 with local-disk fallback |
| **Auth** | JWT, bcrypt, Google OAuth 2.0 |
| **AI** | Groq (compound, LLaMA 3.2 Vision), Google Gemini |
| **Deploy** | Docker on Hugging Face Spaces |

---

## Core Features

**Storage Engine**
- SHA-256 content-addressable blobs with reference-counted deduplication
- Streaming upload: hash + MIME detect + size validate in one pass
- S3 production storage with hash-sharded local fallback

**Auth & Security**
- JWT + bcrypt (cost 12) with Google OAuth (server-side token verification)
- Three-way Google login: existing user / email linking / new account
- Per-user rate limiting (token bucket) and storage quota enforcement
- CORS + HSTS + security headers

**AI Features**
- Provider-agnostic dispatch (Groq / Gemini) with auto-detection
- Auto-tagging: tags, descriptions, folder suggestions per file
- Image analysis via LLaMA 3.2 Vision (base64 payloads)
- Document summarization with map-reduce chunking for large docs (>120K chars)
- Iterative refinement with version history (up to 10 entries)
- Per-user daily rate limits (24h sliding window)

**File Management**
- Upload, download, nested folders, soft-delete with trash/restore
- Public share links for files and folders
- Format conversion (TXT/CSV/MD/HTML -> PDF/XLSX) with background jobs

**Admin**
- Platform-wide stats, per-user storage breakdown
- User quota management and account suspension

---

## Project Structure

```
src/
├── main.go                    # Entry point, graceful shutdown
├── api/                       # REST handlers
│   ├── auth_handlers.go       #   Signup, login, Google OAuth
│   ├── files_handlers.go      #   File CRUD, upload, download, AI triggers
│   ├── folders_handlers.go    #   Folder CRUD, sharing
│   ├── summary_handlers.go    #   AI summary + refinement
│   ├── conversion_handlers.go #   File format conversion
│   ├── admin_handlers.go      #   Admin operations
│   ├── stats_handlers.go      #   User statistics
│   ├── public_*.go            #   Unauthenticated access
│   └── middleware/limits.go   #   Rate limiter + quota
├── services/                  # Business logic
│   ├── auth_service.go        #   JWT, bcrypt, Google OAuth
│   ├── storage_service.go     #   S3/local blobs, dedup, MIME validation
│   ├── file_service.go        #   File metadata, soft-delete
│   ├── folder_service.go      #   Folder hierarchy, cascades
│   ├── ai_tag_service.go      #   Groq/Gemini tagging dispatch
│   ├── ai_summary_service.go  #   Summarization, chunking, refinement
│   ├── conversion_service.go  #   Format conversion orchestration
│   └── converters.go          #   Converters (PDF, XLSX)
├── models/                    # Data models (User, File, Blob, Folder, etc.)
├── graphql/                   # GraphQL schema, resolvers, codegen
├── internal/
│   ├── app/app.go             #   Bootstrap, DI, routing, middleware
│   └── db/db.go               #   Connection pool, migration runner
├── migrations/                # 14 ordered SQL migrations (001_ - 014_)
└── swaggerdocs/               # Auto-generated OpenAPI specs
```

---

## API Overview

| Area | Endpoints | Key Operations |
|---|---|---|
| **Auth** | `/auth/signup`, `/auth/login`, `/auth/google` | Register, login, Google OAuth |
| **Files** | `/files`, `/files/{id}`, `/files/{id}/download` | Upload, list, download, delete, move |
| **AI Tags** | `/files/{id}/ai-tags`, `/files/{id}/ai-describe` | Auto-tag, describe, bulk tag |
| **AI Summary** | `/files/{id}/ai-summary`, `.../refine` | Summarize, refine, get status |
| **Folders** | `/folders`, `/folders/{id}/share` | CRUD, share links |
| **Conversions** | `/files/{id}/convert`, `/conversions/{jobId}` | Convert format, poll status, download |
| **Public** | `/public/files/{id}`, `/p/{token}` | Unauthenticated file/folder access |
| **Admin** | `/admin/stats`, `/admin/files`, `/admin/users/{id}/quota` | Stats, file oversight, quota mgmt |
| **GraphQL** | `/graphql`, `/graphql/playground` | Flexible queries and mutations |

All REST routes are under `/api/v1/`. Full details at `/swagger/`.

---

## Getting Started

```bash
# Clone
git clone https://github.com/YOUR_USERNAME/securevault-backend.git
cd securevault-backend

# Configure
cp .env.example .env    # Edit with your DB, JWT, and API keys

# Run
go mod download
go run ./src            # Starts on :8080, runs migrations automatically

# Verify
curl http://localhost:8080/health
```

### Key Environment Variables

| Variable | Required | Description |
|---|---|---|
| `DB_URL` | Yes | PostgreSQL connection string |
| `JWT_SECRET` | Yes (prod) | JWT signing key |
| `GROQ_API_KEY` | No | Enables AI features |
| `GOOGLE_CLIENT_ID` | No | Enables Google OAuth |
| `S3_BUCKET_NAME` | No | Enables S3 storage (local fallback if unset) |
| `CORS_ALLOWED_ORIGIN` | No | Frontend origin (`*` = all) |
| `PORT` | No | Server port (default: `8080`) |

Full reference in `.env.example`.

### Docker

```bash
docker build -t securevault-backend .
docker run -p 8080:8080 --env-file .env securevault-backend
```

---

## Architecture

```
HTTP Server (Gorilla Mux + Middleware)
 ├── REST Handlers ──┐
 ├── GraphQL ────────┤──▶ Service Layer
 └── Swagger UI      │     ├── AuthService ────▶ JWT / bcrypt / Google
                     │     ├── StorageService ─▶ S3 / Local Disk
                     │     ├── FileService ────▶ PostgreSQL (Neon)
                     │     ├── AiTagService ───▶ Groq / Gemini APIs
                     │     ├── AiSummaryService ▶ Groq (map-reduce)
                     │     └── ConversionService ▶ Local converters
```

**Design decisions:**
- **Content-addressable storage** -- blobs keyed by SHA-256 hash, deduped via ref counting
- **Provider-agnostic AI** -- `callAI()` dispatches to Groq or Gemini; adding providers is one method
- **Hybrid API** -- GraphQL for queries, REST for binary ops (upload/download)
- **Dependency injection** -- all services wired in `NewApp()`, no globals

---


