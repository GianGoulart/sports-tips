# Betting Intelligence Agent — Phase 4: Deployment Plan

**Status:** In progress  
**Date:** 2026-05-11

---

## Goal

Get the system running end-to-end on Railway with GitHub Actions CI, then add dashboard + alerts.

---

## Railway State (as of 2026-05-11)

| Service | ID | Status | Notes |
|---------|-----|--------|-------|
| `postgres` | `575f62a8` | CRASHED | needs env vars fixed |
| `api` | `03ba04f0` | No deployment | domain ready, vars set, no repo connected |
| `ml` | `37c425d3` | No deployment | no repo connected |

Vars set on `api`: `ODDS_API_KEY`, `JWT_SECRET`, `SERVER_PORT=8080`  
Missing: `DATABASE_URL` reference (blocked by no GitHub repo)

---

## Tasks

### 1. GitHub repo setup

- [ ] `brew install gh`
- [ ] `gh auth login`
- [ ] `gh repo create sportstips --private --source=. --push`

### 2. Wire Railway services

- [ ] Set `DATABASE_URL = ${{postgres.DATABASE_URL}}` reference on `api`
- [ ] Set `DATABASE_URL = ${{postgres.DATABASE_URL}}` reference on `ml`
- [ ] Set `rootDirectory = ml` on `ml` service config
- [ ] Fix postgres crash (check Railway logs for root cause)
- [ ] Verify all 3 services deploy green

### 3. GitHub Actions CI pipeline

File: `.github/workflows/deploy.yml`

```yaml
name: CI/CD

on:
  push:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.25'
      - run: go test ./...

  deploy:
    needs: test
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Deploy to Railway
        run: |
          npm install -g @railway/cli
          railway up --service api --detach
        env:
          RAILWAY_TOKEN: ${{ secrets.RAILWAY_TOKEN }}
```

- [ ] Add `RAILWAY_TOKEN` secret to GitHub repo
- [ ] Create `.github/workflows/deploy.yml`
- [ ] Verify pipeline runs green on push

### 4. Alerts

- [ ] Create Telegram bot via @BotFather
- [ ] Set `TELEGRAM_BOT_TOKEN` + `TELEGRAM_CHAT_ID` on `api` service
- [ ] Test alert fires on arb signal

### 5. Dashboard (deferred — Phase 5)

- Simple read-only frontend showing live odds, signals, predictions
- Tech TBD (Next.js or plain HTML served by Go)
