build:
	@ bash download.sh --sync --install 4.3.1
	@docker build --platform "linux/arm64" --output "./bin"  .

build-armv7:
	# bash download.sh --sync --install 4.3.1
	@docker build --platform "linux/arm/v7" -f armv7.Dockerfile --output "./bin"  .
