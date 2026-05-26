# Markdown fixture for parity testing
# Demonstrates various markdown syntax patterns

# Sample Project Documentation

## Overview

This document provides comprehensive information about the project, including setup instructions, usage examples, and development guidelines.

## Table of Contents

1. [Getting Started](#getting-started)
2. [Configuration](#configuration)
3. [API Reference](#api-reference)
4. [Examples](#examples)

## Getting Started

### Prerequisites

- Go 1.21 or higher
- PostgreSQL 15+
- Redis 7+

### Installation

Clone the repository and install dependencies:

```bash
git clone https://github.com/example/project.git
cd project
go mod download
```

Run the development server:

```bash
make dev
```

The application will be available at `http://localhost:8080`.

## Configuration

Configuration is managed through environment variables. Reference `.env.example` for available options:

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `PORT` | Server port | `8080` | No |
| `DATABASE_URL` | PostgreSQL connection string | - | Yes |
| `REDIS_URL` | Redis connection string | - | Yes |
| `LOG_LEVEL` | Logging verbosity | `INFO` | No |

### Database Schema

The project uses the following database schema:

```sql
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    username VARCHAR(255) UNIQUE NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

## API Reference

### Authentication

All API endpoints require authentication using a Bearer token:

```
Authorization: Bearer <your-token>
```

### Endpoints

#### GET /api/users

Retrieve a list of users.

**Response:**

```json
{
  "users": [
    {
      "id": 1,
      "username": "alice",
      "email": "alice@example.com"
    }
  ]
}
```

> **Note:** Results are paginated with a default limit of 20 items per page.

## Examples

### Example 1: Creating a User

```go
user := User{
    Username: "alice",
    Email:    "alice@example.com",
}

err := db.Create(&user).Error
// Handle error
```

### Example 2: Fetching Data

```bash
curl -H "Authorization: Bearer $TOKEN" \
     http://localhost:8080/api/users
```

---

*Documentation last updated: 2026-02-26*