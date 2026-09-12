.PHONY: build build-armv7 test install-objectbox

build:
	# bash download.sh --sync --install 4.3.1
	@docker build --platform "linux/arm64" --output "./bin"  .

build-armv7:
	# bash download.sh --sync --install 4.3.1
	@docker build --platform "linux/arm/v7" -f armv7.Dockerfile --output "./bin"  .

test:
	@go test ./...
	
install-objectbox:
	@tar -xzf objectbox-linux-*.tar.gz -C lib --strip-components=1 lib/libobjectbox.so
