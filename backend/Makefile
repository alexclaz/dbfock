.PHONY: prepare-desktop-icon dev-desktop build-desktop

prepare-desktop-icon:
	mkdir -p build
	cp appicon.png build/appicon.png

dev-desktop: prepare-desktop-icon
	go run github.com/wailsapp/wails/v2/cmd/wails@v2.10.1 dev

build-desktop: prepare-desktop-icon
	go run github.com/wailsapp/wails/v2/cmd/wails@v2.10.1 build
