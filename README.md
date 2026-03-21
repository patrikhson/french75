# French 75 Tracker

A cocktail check-in web app for tracking classic cocktail experiences.

## Quick start (local development)

### 1. Check your tools
```bash
bash scripts/recon-local.sh
```
You need: Go 1.22+, git, gh (GitHub CLI), node/npm, psql.

### 2. Install Go dependencies
```bash
go mod tidy
```

### 3. Set up local PostgreSQL
```bash
createdb french75
```

### 4. Configure environment
```bash
cp .env.example .env
# Edit .env — at minimum set DATABASE_URL
```

### 5. Run the server
```bash
go run ./cmd/server
```
Visit http://localhost:8080/health — should return `ok`.

### 6. Build CSS (for frontend work)
```bash
npm install
npx tailwindcss -i ./static/css/input.css -o ./static/css/app.css --watch
```

---

## Deployment

See `scripts/` for all VM setup scripts. The general flow is:

1. Push code to `main` branch on GitHub
2. GitHub Actions builds the binary and deploys to vm01 via SSH

For first-time VM setup, see `scripts/setup-vm04.sh` and `scripts/setup-vm01.sh`.
