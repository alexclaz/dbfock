.PHONY: dev-backend dev-frontend prepare-desktop-icon dev-desktop test typecheck build build-desktop docker-up

dev-backend:
	cd backend && ENCRYPTION_KEY=local-development-key go run ./cmd/api

dev-frontend:
	cd frontend && npm run dev

prepare-desktop-icon:
	mkdir -p backend/build
	cp frontend/public/branding/favicon/android-chrome-512x512.png backend/build/appicon.png

dev-desktop: prepare-desktop-icon
	cd backend && go run github.com/wailsapp/wails/v2/cmd/wails@v2.10.1 dev

test:
	cd backend && go test ./...

typecheck:
	cd frontend && npm run typecheck

build:
	cd backend && go build ./cmd/api
	cd frontend && npm run build

build-desktop: prepare-desktop-icon
	cd backend && go run github.com/wailsapp/wails/v2/cmd/wails@v2.10.1 build

docker-up:
	docker compose up --build
