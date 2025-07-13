.PHONY: dev build clean setup

# Development mode with hot reloading
dev:
	@echo "🚀 Starting development server..."
	@$(shell go env GOPATH)/bin/templ generate
	@$(shell go env GOPATH)/bin/air

# Build for production
build:
	@echo "🏭 Building for production..."
	@$(shell go env GOPATH)/bin/templ generate
	@go build -o mini-blog .

# Setup environment
setup:
	@echo "⚙️  Setting up environment..."
	@[ -f .env ] || cp env.example .env
	@echo "✅ .env file ready - edit with your database credentials"

# Clean build artifacts
clean:
	@rm -f mini-blog
	@rm -rf tmp/
	@echo "🧹 Cleaned build artifacts"

# Install tools
install-tools:
	@echo "📦 Installing development tools..."
	@go install github.com/a-h/templ/cmd/templ@latest
	@go install github.com/air-verse/air@latest
	@echo "✅ Tools installed" 