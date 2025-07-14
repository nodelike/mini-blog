.PHONY: dev build clean setup

# Development mode with hot reloading
dev:
	@echo "🚀 Starting development server..."
	@$(shell go env GOPATH)/bin/air


# Build for production
build:
	@echo "🏭 Building for production..."
	@$(shell go env GOPATH)/bin/templ generate
	@npm run build
	@go build -o mini-blog .

# Build CSS only
build-css:
	@echo "🎨 Building CSS..."
	@npm run build

# Watch CSS for changes
watch-css:
	@echo "👀 Watching CSS for changes..."
	@npm run build-css

# Setup environment
setup:
	@echo "⚙️  Setting up environment..."
	@[ -f .env ] || cp env.example .env
	@npm install
	@echo "✅ .env file ready - edit with your database credentials"
	@echo "✅ Dependencies installed"

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
	@npm install
	@echo "✅ Tools installed" 