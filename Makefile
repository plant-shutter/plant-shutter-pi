.PHONY: build build-armv7 frontend-build package deploy deploy-backend stop test install-objectbox

DEPLOY_HOST ?= root@192.168.2.91
DEPLOY_DIR ?= /home/vincent/plant-shutter
PACKAGE_FILE ?= bin/plant-shutter-aarch64.tar.gz
BACKEND_PACKAGE_FILE ?= bin/plant-shutter-backend-aarch64.tar.gz

build:
	@docker build --platform "linux/arm64" --output "./bin"  .

build-armv7:
	@docker build --platform "linux/arm/v7" -f armv7.Dockerfile --output "./bin"  .

frontend-build:
	@npm --prefix frontend run build

package: build frontend-build
	@set -eu; \
	stage=$$(mktemp -d); \
	trap 'rm -rf "$$stage"' EXIT; \
	mkdir -p "$$stage/bin" "$$stage/lib" "$$stage/static"; \
	cp bin/plant-shutter "$$stage/bin/"; \
	cp bin/lib/libobjectbox.so "$$stage/lib/"; \
	cp -a frontend/dist/. "$$stage/static/"; \
	cp scripts/run.sh "$$stage/run.sh"; \
	chmod +x "$$stage/run.sh"; \
	printf '%s\n' 'Plant Shutter ARM64 package' '' 'Start manually: ./run.sh' 'Install as a systemd service: sudo ./run.sh install' 'Logs are written to plant-shutter.log when installed as a service.' '' 'The package targets 64-bit Raspberry Pi (aarch64).' > "$$stage/README.txt"; \
	tar -czf bin/plant-shutter-aarch64.tar.gz -C "$$stage" .; \
	echo "Created bin/plant-shutter-aarch64.tar.gz"

deploy: package stop
	@set -eu; \
	 scp "$(PACKAGE_FILE)" "$(DEPLOY_HOST):/tmp/plant-shutter-aarch64.tar.gz"; \
	 ssh "$(DEPLOY_HOST)" 'mkdir -p "$(DEPLOY_DIR)" && tar -xzf /tmp/plant-shutter-aarch64.tar.gz -C "$(DEPLOY_DIR)" && cd "$(DEPLOY_DIR)" && (nohup ./run.sh > "$(DEPLOY_DIR)/plant-shutter.log" 2>&1 </dev/null &)'; \
	 echo "Deployed to $(DEPLOY_HOST):$(DEPLOY_DIR)"

deploy-backend: build stop
	@set -eu; \
	 stage=$$(mktemp -d); \
	 trap 'rm -rf "$$stage"' EXIT; \
	 mkdir -p "$$stage/bin" "$$stage/lib" "$$stage/static"; \
	 cp bin/plant-shutter "$$stage/bin/"; \
	 cp bin/lib/libobjectbox.so "$$stage/lib/"; \
	 cp scripts/run.sh "$$stage/run.sh"; \
	 chmod +x "$$stage/run.sh"; \
	 printf '%s\n' 'Plant Shutter backend ARM64 package' '' 'Start manually: ./run.sh' 'Install as a systemd service: sudo ./run.sh install' 'The backend package includes an empty static directory; run the frontend separately or use the full package.' > "$$stage/README.txt"; \
	 tar -czf "$(BACKEND_PACKAGE_FILE)" -C "$$stage" .; \
	 scp "$(BACKEND_PACKAGE_FILE)" "$(DEPLOY_HOST):/tmp/plant-shutter-backend-aarch64.tar.gz"; \
	 ssh "$(DEPLOY_HOST)" 'set -eu; mkdir -p "$(DEPLOY_DIR)"; tar -xzf /tmp/plant-shutter-backend-aarch64.tar.gz -C "$(DEPLOY_DIR)"; cd "$(DEPLOY_DIR)"; if systemctl cat plant-shutter.service >/dev/null 2>&1; then systemctl enable --now plant-shutter.service; else nohup ./run.sh >> "$(DEPLOY_DIR)/plant-shutter.log" 2>&1 </dev/null & fi'; \
	 echo "Backend deployed and started in background on $(DEPLOY_HOST):$(DEPLOY_DIR)"; \
	 echo "Log: $(DEPLOY_DIR)/plant-shutter.log"

stop:
	@ssh "$(DEPLOY_HOST)" 'systemctl stop plant-shutter.service 2>/dev/null || true; pids=$$(pgrep -x plant-shutter || true); if [ -n "$$pids" ]; then kill $$pids; echo "Stopped plant-shutter processes: $$pids"; else echo "No plant-shutter processes found"; fi'

test:
	@go test ./...
	
install-objectbox:
	@tar -xzf objectbox-linux-*.tar.gz -C lib --strip-components=1 lib/libobjectbox.so
