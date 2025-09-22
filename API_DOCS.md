# SecureVault Backend - API Documentation

This directory contains the SecureVault backend implementation. For comprehensive API documentation, please refer to our modular documentation structure.

## 📖 API Documentation

### Complete API Reference

For detailed API documentation, examples, and guides, visit:

**[📚 Main API Documentation](../apidocs/)**

### Quick Links

| API Type        | Documentation                          | Description                                          |
| --------------- | -------------------------------------- | ---------------------------------------------------- |
| **REST API**    | [📖 REST Docs](../apidocs/rest/)       | Traditional REST endpoints with full CRUD operations |
| **GraphQL API** | [📖 GraphQL Docs](../apidocs/graphql/) | Modern GraphQL interface with flexible queries       |

### Specific REST Modules

- [🔐 Authentication](../apidocs/rest/auth.md) - User registration and login
- [📁 File Management](../apidocs/rest/files.md) - Upload, organize, and share files
- [📂 Folder Management](../apidocs/rest/folders.md) - Hierarchical folder structure
- [🌐 Public Access](../apidocs/rest/public.md) - Anonymous file and folder sharing
- [📊 Statistics](../apidocs/rest/stats.md) - Usage analytics and quotas
- [👑 Admin Operations](../apidocs/rest/admin.md) - System administration
- [📄 cURL Examples](../apidocs/rest/examples.md) - Comprehensive command-line examples

## 🚀 Quick Start

### 1. Start the Server

```bash
cd backend
go run src/main.go
```

### 2. Access API Documentation

- **REST Swagger UI**: http://localhost:8080/swagger/index.html
- **GraphQL Playground**: http://localhost:8080/api/v1/graphql/playground

### 3. Health Check

```bash
curl http://localhost:8080/health
# Response: {"status":"ok","service":"securevault-backend"}
```

### 4. Basic Authentication

```bash
# Register new user
curl -X POST http://localhost:8080/api/v1/auth/signup \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"secure123","full_name":"John Doe"}'

# Login existing user
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"secure123"}'
```

## 🏗️ Backend Architecture

### Directory Structure

```
backend/
├── src/
│   ├── main.go              # Application entry point
│   ├── api/                 # REST API handlers
│   │   ├── *_handlers.go    # Endpoint implementations
│   │   └── middleware/      # HTTP middleware
│   ├── graphql/             # GraphQL API implementation
│   │   ├── server.go        # GraphQL server setup
│   │   ├── graph/           # Schema and resolvers
│   │   └── middleware/      # GraphQL middleware
│   ├── internal/
│   │   ├── app/             # Application configuration
│   │   └── db/              # Database connection
│   ├── models/              # Data models
│   ├── services/            # Business logic
│   ├── migrations/          # Database migrations
│   └── swaggerdocs/         # API documentation
├── storage/                 # Local file storage
├── tests/                   # Test suites
└── test-files/              # Test data
```

### Key Components

#### API Layers

- **REST API**: Traditional HTTP endpoints at `/api/v1/*`
- **GraphQL API**: Modern query interface at `/api/v1/graphql`
- **Public Access**: Anonymous endpoints at `/p/*`

#### Services Layer

- **AuthService**: User authentication and JWT management
- **FileService**: File operations and metadata management
- **FolderService**: Hierarchical folder operations
- **StatsService**: Analytics and usage statistics
- **StorageService**: File storage abstraction (local/S3)

#### Data Layer

- **PostgreSQL**: User data, file metadata, folder structure
- **File Storage**: Local filesystem or S3-compatible storage
- **Caching**: In-memory caching for frequently accessed data

## 🔧 Development Setup

### Prerequisites

- Go 1.19+
- PostgreSQL 12+
- (Optional) S3-compatible storage

### Environment Configuration

Create a `.env` file in the backend directory:

```env

# Server Configuration
PORT=8080

# Database Configuration (NeonDB)
DB_URL=your_psql_db_url

# AWS S3 Configuration (bucket)
AWS_ACCESS_KEY_ID=your_aws_access_key_id
AWS_SECRET_ACCESS_KEY=your_aws_secret_access_key
AWS_REGION=your_aws_region
S3_BUCKET_NAME=your_bucket_name

# JWT Authentication
JWT_SECRET=your_secret_key_for_encryption

# Rate Limiting (requests per second per user)
RATE_LIMIT_RPS=5

# Storage Quota (bytes per user: 10 MB)
QUOTA_BYTES=10485760

# Environment
ENVIRONMENT=development

# CORS Configuration (development only)
CORS_ALLOW_ALL=true

```

### Running the Server

```bash
# Install dependencies
go mod download

# Run database migrations
go run src/main.go migrate

# Start the server
go run src/main.go
```

### Running Tests

```bash
# Unit tests
go test ./...

# Integration tests
go test ./tests/integration/

# Contract tests (includes GraphQL test suite)
go test ./tests/contract/
```

## 📊 API Endpoints Overview

### REST API Base URL

```
http://localhost:8080/api/v1
```

### Core Endpoints

| Category    | Endpoints                     | Description               |
| ----------- | ----------------------------- | ------------------------- |
| **Auth**    | `/auth/signup`, `/auth/login` | User authentication       |
| **Files**   | `/files/*`                    | File CRUD operations      |
| **Folders** | `/folders/*`                  | Folder management         |
| **Public**  | `/p/*`, `/public/*`           | Anonymous access          |
| **Stats**   | `/stats/me`                   | User analytics            |
| **Admin**   | `/admin/*`                    | Administrative operations |

### GraphQL Endpoint

| Type           | Endpoint                         | Description               |
| -------------- | -------------------------------- | ------------------------- |
| **API**        | `POST /api/v1/graphql`           | GraphQL operations        |
| **Playground** | `GET /api/v1/graphql/playground` | Interactive query builder |

## 🔒 Security Features

### Authentication & Authorization

- **JWT Tokens**: Stateless authentication
- **Role-Based Access**: User/admin permissions
- **Request Validation**: Input sanitization

### File Security

- **User Isolation**: Secure file ownership
- **Public Sharing**: Token-based sharing
- **Admin Override**: Administrative access

### Rate Limiting

- **Per-User Throttling**: Configurable limits
- **Abuse Prevention**: Request monitoring
- **Quota Management**: Storage limits

## 🚦 Deployment

### Production Checklist

- [ ] Configure production database
- [ ] Set up file storage (S3 recommended)
- [ ] Configure environment variables
- [ ] Set up HTTPS/TLS
- [ ] Configure rate limiting
- [ ] Set up monitoring and logging
- [ ] Run security audit

### Docker Deployment

```bash
# Build and run with docker-compose
docker-compose up -d
```

### Environment Variables

See `.env.example` for complete configuration options.

## 📋 Testing

### Test Suites

- **Unit Tests**: Individual component testing
- **Integration Tests**: Database and service integration
- **Contract Tests**: API contract validation
- **GraphQL Test Suite**: Comprehensive GraphQL API testing

### Running Specific Tests

```bash
# GraphQL test suite
go test ./tests/contract/ -v -run TestGraphQL

# File operations
go test ./tests/integration/ -v -run TestFile

# Admin operations
go test ./tests/contract/ -v -run TestAdmin
```

## 🆘 Troubleshooting

### Common Issues

1. **Database Connection**: Check PostgreSQL is running and credentials are correct
2. **File Upload Errors**: Verify storage directory permissions
3. **JWT Errors**: Ensure JWT_SECRET is set and consistent
4. **CORS Issues**: Check CORS configuration for frontend integration

### Debug Mode

```bash
# Run with debug logging
DEBUG=true go run src/main.go
```

### Health Checks

```bash
# Server health
curl http://localhost:8080/health

# Database connectivity
curl http://localhost:8080/api/v1/stats/me -H "Authorization: Bearer TOKEN"
```

## 📚 Additional Resources

- **[Complete API Documentation](../apidocs/)** - Comprehensive API reference
- **[GraphQL Schema](src/graphql/graph/schema.graphqls)** - GraphQL type definitions
- **[Database Migrations](src/migrations/)** - Database schema evolution
- **[Test Examples](tests/)** - Test implementation examples

---

For detailed API usage examples and comprehensive documentation, visit the **[main API documentation](../apidocs/)**.
