# Mini Blog

A simple and clean blog application built with Go, Templ, HTMX, and Tailwind CSS.

## Tech Stack

- **Backend**: Go with Echo framework
- **Frontend**: Templ (HTML templating), HTMX, Tailwind CSS
- **Database**: PostgreSQL with GORM
- **Development**: Air for hot reloading

## Features

- Clean, modern UI with Tailwind CSS
- HTMX for dynamic interactions
- Admin interface for managing posts
- Responsive design
- Hot reloading during development

## Prerequisites

- Go 1.19+
- PostgreSQL

## Quick Start

```bash
# 1. Create database
createdb mini_blog

# 2. Install tools
make install-tools

# 3. Setup environment
make setup
# Edit .env with your database credentials and admin email

# 4. Start development server
make dev
```

### Environment Configuration

After running `make setup`, edit the `.env` file with your settings:

```env
# Database
DB_HOST=localhost
DB_PORT=5432
DB_USER=nodelike
DB_PASSWORD=
DB_NAME=mini_blog

# Authentication
ADMIN_EMAIL=your-admin@email.com    # This user will be auto-assigned admin role
RESEND_API_KEY=your-resend-api-key  # Optional: For email functionality

# Security
JWT_SECRET=your-jwt-secret
SESSION_KEY=your-session-key-32-chars

# Server
PORT=8080
```

### Default Admin User

- If you set `ADMIN_EMAIL` in `.env`, that user will automatically become admin
- Default password for initial admin: `admin123` (change after first login)
- Without `RESEND_API_KEY`, OTP codes will be shown in console (development only)

## Commands

```bash
make dev          # Start development server with hot reload
make build        # Build for production
make setup        # Create .env file from template
make clean        # Clean build artifacts
make install-tools # Install templ and air tools
```

## Usage

- **Home Page**: `http://localhost:8080/` - Shows latest published posts
- **All Posts**: `http://localhost:8080/posts` - Shows all published posts
- **Individual Post**: `http://localhost:8080/posts/{slug}` - Shows a specific post
- **Admin Interface**: `http://localhost:8080/admin/posts` - Manage posts (create, edit, delete)

## Project Structure

```
mini-blog/
├── app/
│   ├── config/       # Configuration
│   ├── handlers/     # HTTP handlers
│   ├── models/       # Database models
│   └── templates/    # Templ templates
├── main.go          # Application entry point
├── Makefile         # Build commands
└── .env             # Environment config
```

## Features

- **Public Blog**: View posts, individual post pages
- **User Authentication**: Sign up, login, email verification with OTP
- **Admin Interface**: Create, edit, delete posts with HTMX (protected)
- **Email Integration**: OTP verification and welcome emails via Resend
- **Role-Based Access**: Admin users automatically assigned from config
- **Hot Reloading**: Automatic restart during development
- **Clean UI**: Modern design with Tailwind CSS

## Routes

**Public Routes:**
- `/` - Home page with latest posts
- `/posts` - All published posts
- `/posts/:slug` - Individual post view
- `/signup` - User registration with email verification
- `/login` - User login
- `/logout` - User logout

**Protected Routes:**
- `/admin/posts` - Admin interface for managing posts (admin only) 