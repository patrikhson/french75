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

### Directory layout on servers
- `/usr/local/src/french75` — git clone on each VM (source code, setup scripts)
- `/opt/french75/` — runtime directory on vm01: binary, `.env`, `photos/`

### First-time VM setup
```bash
# On each VM — clone source to /usr/local/src
sudo git clone https://github.com/patrikhson/french75.git /usr/local/src/french75

# On vm04
bash /usr/local/src/french75/scripts/setup-vm04.sh <vm01-ip> <db-password>

# On vm01
bash /usr/local/src/french75/scripts/setup-vm01.sh french75.paftech.se 8090
```

### Ongoing deployments
Push to `master` — GitHub Actions builds the binary and deploys to vm01 via SSH automatically.

### Update setup scripts on a VM
```bash
cd /usr/local/src/french75 && sudo git pull
```
