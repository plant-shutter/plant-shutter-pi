FROM --platform=$BUILDPLATFORM registry.cn-hangzhou.aliyuncs.com/adpc/goxx:latest AS build

ENV OUTPUT="plant-shutter"
ENV CGO_ENABLED=1
WORKDIR /src

ARG TARGETPLATFORM
RUN --mount=type=cache,sharing=private,target=/var/cache/apt \
  --mount=type=cache,sharing=private,target=/var/lib/apt/lists \
  goxx-apt-get install -y gcc-arm-linux-gnueabi binutils gcc g++ pkg-config wget

RUN wget https://raw.githubusercontent.com/objectbox/objectbox-c/main/download.sh
RUN bash download.sh --sync --install 4.3.1 Linux aarch64 || true


RUN --mount=type=bind,source=. \
  --mount=type=cache,target=/root/.cache \
  --mount=type=cache,target=/go/pkg/mod \
  export GOPROXY=https://proxy.golang.com.cn && \
  export CC=arm-linux-gnueabihf-gcc && \
#  goxx-go build -o /out/${OUTPUT} main.go
#  goxx-go build -o /out/${OUTPUT} cmd/preview-test/main.go
#  goxx-go build -o /out/${OUTPUT} cmd/camera/main.go
#  goxx-go build -o /out/${OUTPUT} cmd/test/main.go
#  goxx-go build -o /out/${OUTPUT} cmd/camera-fast/main.go
  goxx-go build -o /out/${OUTPUT} cmd/h264-raw/main.go

FROM scratch AS artifact
COPY --from=build /out /

## Build with the following command
# docker build --platform "linux/arm/v6" --output "./bin"  .
