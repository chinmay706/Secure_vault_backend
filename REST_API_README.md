# SecureVault REST API Documentation

This document provides comprehensive documentation for the SecureVault REST API endpoints.

## Base URL

```
http://localhost:8080/api/v1
```

## Authentication

Most endpoints require Bearer token authentication:

```
Authorization: Bearer <jwt_token>
```

## Public Endpoints (No Authentication Required)

### Health Check

- **GET** `/health` - Check service health

### Authentication

- **POST** `/auth/signup` - User registration
- **POST** `/auth/login` - User login

### Public File Access

- **GET** `/public/files/{id}` - Get public file details by ID
- **GET** `/public/files/share/{token}` - Get file details by share token
- **GET** `/public/folders/share/{token}` - Get complete folder tree with all subfolders and files by share token

### Public File Downloads

- **GET** `/public/files/{id}/download` - Download public file by ID
- **GET** `/public/files/share/{token}/download` - Download file by share token

### Public Downloads (Legacy)

- **GET** `/p/{token}` - Download public file by share token
- **HEAD** `/p/{token}` - Get public file headers
- **GET** `/p/f/{token}` - Access public folder contents by share token

## Authenticated Endpoints

### Files

- **GET** `/files` - List user files with filtering and pagination
- **POST** `/files` - Upload a new file
- **GET** `/files/{id}` - Get file details
- **DELETE** `/files/{id}` - Delete a file
- **GET** `/files/{id}/download` - Download a file
- **PATCH** `/files/{id}/public` - Toggle file public status
- **PATCH** `/files/{id}/move` - Move file to different folder

### Folders

- **POST** `/folders` - Create a new folder
- **GET** `/folders` - List user folders
- **GET** `/folders/{id}` - Get folder details
- **PATCH** `/folders/{id}` - Update folder (rename)
- **DELETE** `/folders/{id}` - Delete folder and contents
- **POST** `/folders/{id}/share` - Create folder share link
- **DELETE** `/folders/{id}/share` - Delete folder share link
- **GET** `/folders/{id}/share/status` - Check if folder has sharelinks

### Statistics

- **GET** `/stats/me` - Get user statistics

### User Management

- **DELETE** `/users/{id}` - Delete own user account
- **PATCH** `/users/{id}/password` - Update own password

### Admin (Admin Role Required)

- **POST** `/admin/signup` - Create new admin user account
- **POST** `/admin/promote` - Promote user to admin role
- **GET** `/admin/files` - List all files (admin view)
- **DELETE** `/admin/files/{id}` - Delete any file (admin)
- **GET** `/admin/stats` - Get system statistics
- **PATCH** `/admin/users/{id}/quota` - Update user quota
- **POST** `/admin/users/{id}/suspend` - Suspend user

## Protected Folder Endpoints

### GET /folders/{id}/share/status

Check if a folder has any active sharelinks (authentication required).

**Headers:**

- `Authorization: Bearer {token}` - Required

**Parameters:**

- `id` (path) - Folder UUID

**Response (200):**

```json
{
  "has_share_link": true,
  "token": "hcHX6GReXl1De2guwIkbdGAali2jVPeYGvcowr4ihac"
}
```

Or when no sharelink exists:

```json
{
  "has_share_link": false
}
```

**Error Responses:**

- `400` - Invalid folder ID format
- `401` - Authentication required
- `404` - Folder not found or access denied
- `500` - Internal server error

**Example:**

```bash
curl -H "Authorization: Bearer YOUR_JWT_TOKEN" \
     "http://localhost:8080/api/v1/folders/d1de045f-8481-4490-8aec-e91269fff086/share/status"
```

## New Public File Access Endpoints

### GET /public/files/{id}

Get file details by ID if the file is public (no authentication required).

**Parameters:**

- `id` (path) - File UUID

**Response (200):**

```json
{
  "id": "91b5c889-c8b2-493b-bfe4-2d01774dce96",
  "original_filename": "files.csv",
  "mime_type": "text/plain; charset=utf-8",
  "size_bytes": 691,
  "tags": [],
  "download_url": "http://localhost:8080/api/v1/public/files/91b5c889-c8b2-493b-bfe4-2d01774dce96/download",
  "created_at": "2025-09-21T17:43:42Z",
  "updated_at": "2025-09-21T19:04:43Z",
  "folder_id": null
}
```

**Error Responses:**

- `400` - Invalid file ID format
- `403` - File is private
- `404` - File not found or not public
- `500` - Internal server error

**Example:**

```bash
curl "http://localhost:8080/api/v1/public/files/91b5c889-c8b2-493b-bfe4-2d01774dce96"
```

### GET /public/files/share/{token}

Get file details using a share token (no authentication required).

**Parameters:**

- `token` (path) - File share token

**Response (200):**

```json
{
  "id": "91b5c889-c8b2-493b-bfe4-2d01774dce96",
  "original_filename": "files.csv",
  "mime_type": "text/plain; charset=utf-8",
  "size_bytes": 691,
  "tags": [],
  "download_url": "http://localhost:8080/api/v1/public/files/share/hcHX6GReXl1De2guwIkbdGAali2jVPeYGvcowr4ihac/download",
  "created_at": "2025-09-21T17:43:42Z",
  "updated_at": "2025-09-21T19:04:43Z",
  "folder_id": null
}
```

**Error Responses:**

- `400` - Invalid token format
- `403` - File is private or link revoked
- `404` - Share link not found
- `410` - Share link expired
- `500` - Internal server error

**Example:**

```bash
curl "http://localhost:8080/api/v1/public/files/share/hcHX6GReXl1De2guwIkbdGAali2jVPeYGvcowr4ihac"
```

### GET /public/folders/share/{token}

Get complete folder tree structure using a share token (no authentication required). Returns the folder along with all its subfolders and files in a hierarchical tree format.

**Parameters:**

- `token` (path) - Folder share token

**Response (200):**

```json
{
  "folder": {
    "id": "d1de045f-8481-4490-8aec-e91269fff086",
    "name": "Documents",
    "parent_id": null,
    "created_at": "2025-09-21T08:10:29Z",
    "updated_at": "2025-09-21T15:15:52Z"
  },
  "files": [
    {
      "id": "f1de045f-8481-4490-8aec-e91269fff087",
      "original_filename": "report.pdf",
      "mime_type": "application/pdf",
      "size_bytes": 1024000,
      "tags": ["important", "2024"],
      "download_url": "http://localhost:8080/api/v1/public/files/f1de045f-8481-4490-8aec-e91269fff087/download",
      "created_at": "2025-09-21T08:15:29Z",
      "updated_at": "2025-09-21T08:15:29Z"
    }
  ],
  "subfolders": [
    {
      "folder": {
        "id": "d2de045f-8481-4490-8aec-e91269fff088",
        "name": "Subfolder",
        "parent_id": "d1de045f-8481-4490-8aec-e91269fff086",
        "created_at": "2025-09-21T09:10:29Z",
        "updated_at": "2025-09-21T09:10:29Z"
      },
      "files": [
        {
          "id": "f2de045f-8481-4490-8aec-e91269fff089",
          "original_filename": "nested.txt",
          "mime_type": "text/plain",
          "size_bytes": 2048,
          "tags": [],
          "download_url": "http://localhost:8080/api/v1/public/files/f2de045f-8481-4490-8aec-e91269fff089/download",
          "created_at": "2025-09-21T09:15:29Z",
          "updated_at": "2025-09-21T09:15:29Z"
        }
      ],
      "subfolders": []
    }
  ]
}
```

**Error Responses:**

- `400` - Invalid token format
- `404` - Share link not found
- `410` - Share link expired
- `500` - Internal server error

**Example:**

```bash
curl "http://localhost:8080/api/v1/public/folders/share/folder-share-token-here"
```

## Public File Download Endpoints

### GET /public/files/{id}/download

Download public file content by file ID (no authentication required).

**Parameters:**

- `id` (path) - File ID (UUID)

**Response (200):** File content with appropriate headers

**Error Responses:**

- `400` - Invalid file ID format
- `403` - File is not public
- `404` - File not found
- `500` - Download failed

**Example:**

```bash
curl "http://localhost:8080/api/v1/public/files/91b5c889-c8b2-493b-bfe4-2d01774dce96/download" -o downloaded_file.csv
```

### GET /public/files/share/{token}/download

Download file content using share token (no authentication required).

**Parameters:**

- `token` (path) - File share token

**Response (200):** File content with appropriate headers

**Error Responses:**

- `400` - Invalid token format
- `403` - Share link expired or inactive
- `404` - File not found
- `500` - Download failed

**Example:**

```bash
curl "http://localhost:8080/api/v1/public/files/share/hcHX6GReXl1De2guwIkbdGAali2jVPeYGvcowr4ihac/download" -o downloaded_file.csv
```

## File Upload with Public Sharing Workflow

1. **Upload File** (authenticated):

   ```bash
   curl -X POST "http://localhost:8080/api/v1/files" \
     -H "Authorization: Bearer <token>" \
     -F "file=@document.pdf"
   ```

2. **Make File Public** (authenticated):

   ```bash
   curl -X PATCH "http://localhost:8080/api/v1/files/{id}/public" \
     -H "Authorization: Bearer <token>" \
     -H "Content-Type: application/json" \
     -d '{"is_public": true}'
   ```

3. **Access File Publicly** (no auth):

   ```bash
   # Get file details
   curl "http://localhost:8080/api/v1/public/files/{id}"

   # Or use share token
   curl "http://localhost:8080/api/v1/public/files/share/{token}"

   # Download file
   curl "http://localhost:8080/api/v1/p/{token}"
   ```

## Error Response Format

All endpoints return errors in this format:

```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "Human readable error message"
  }
}
```

Common error codes:

- `INVALID_TOKEN` - Invalid or malformed token
- `NOT_FOUND` - Resource not found
- `PRIVATE_FILE` - File is not publicly accessible
- `EXPIRED` - Share link has expired
- `INACTIVE` - Share link has been deactivated
- `INTERNAL_ERROR` - Server error

## GraphQL Alternative

For more flexible queries, see the GraphQL API documentation in [GRAPHQL_README.md](src/graphql/GRAPHQL_README.md).

## Swagger Documentation

Interactive API documentation is available at:

```
http://localhost:8080/swagger/index.html
```

## Rate Limiting

All endpoints are subject to rate limiting:

- Default: 5 requests per second per IP
- Authenticated users may have higher limits based on their tier
