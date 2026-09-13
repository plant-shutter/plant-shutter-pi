FROM --platform=$BUILDPLATFORM registry.cn-hangzhou.aliyuncs.com/adpc/goxx:latest AS build

ENV OUTPUT="plant-shutter"
ENV CGO_ENABLED=1
WORKDIR /src

ARG TARGETPLATFORM
COPY objectbox-install/v5.3.2/objectbox-linux-armv7hf.tar.gz /tmp/objectbox.tar.gz

RUN --mount=type=cache,sharing=private,target=/var/cache/apt \
  --mount=type=cache,sharing=private,target=/var/lib/apt/lists \
  goxx-apt-get install -y gcc-arm-linux-gnueabihf binutils gcc g++ pkg-config

RUN mkdir -p /opt/objectbox/include /opt/objectbox/lib && \
  tar -xzf /tmp/objectbox.tar.gz -C /opt/objectbox && \
  ln -s /opt/objectbox/lib/libobjectbox.so /usr/lib/arm-linux-gnueabihf/libobjectbox.so


RUN --mount=type=bind,source=. \
  --mount=type=cache,target=/root/.cache \
  --mount=type=cache,target=/go/pkg/mod \
  export GOPROXY=https://proxy.golang.com.cn && \
  export CC=arm-linux-gnueabihf-gcc && \
  export CGO_CFLAGS=-I/opt/objectbox/include && \
  export CGO_LDFLAGS='-L/opt/objectbox/lib' && \
  goxx-go build -o /out/${OUTPUT} main.go && \
  mkdir -p /out/lib && \
  cp -L /opt/objectbox/lib/libobjectbox.so /out/lib/
#  goxx-go build -o /out/${OUTPUT} cmd/preview-test/main.go
#  goxx-go build -o /out/${OUTPUT} cmd/camera/main.go

FROM scratch AS artifact
COPY --from=build /out /

## Build with the following command
# docker build --platform "linux/arm/v6" --output "./bin"  .
