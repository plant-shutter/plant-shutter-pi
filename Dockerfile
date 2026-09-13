FROM --platform=$BUILDPLATFORM registry.cn-hangzhou.aliyuncs.com/adpc/goxx:latest AS build

ENV OUTPUT="plant-shutter"
ENV CGO_ENABLED=1
WORKDIR /src

ARG TARGETPLATFORM
COPY objectbox-install/v5.3.2/objectbox-linux-aarch64.tar.gz /tmp/objectbox.tar.gz

RUN --mount=type=cache,sharing=private,target=/var/cache/apt \
  --mount=type=cache,sharing=private,target=/var/lib/apt/lists \
  goxx-apt-get install -y gcc-aarch64-linux-gnu binutils gcc g++ pkg-config wget

RUN mkdir -p /opt/objectbox/include /opt/objectbox/lib && \
  tar -xzf /tmp/objectbox.tar.gz -C /opt/objectbox && \
  ln -s /opt/objectbox/lib/libobjectbox.so /usr/lib/aarch64-linux-gnu/libobjectbox.so

RUN --mount=type=bind,source=. \
  --mount=type=cache,target=/root/.cache \
  --mount=type=cache,target=/go/pkg/mod \
  export GOPROXY=https://proxy.golang.com.cn && \
  export CC=aarch64-linux-gnu-gcc && \
  export CGO_CFLAGS=-I/opt/objectbox/include && \
  export CGO_LDFLAGS='-L/opt/objectbox/lib' && \
  goxx-go build -o /out/${OUTPUT} main.go && \
  # goxx-go build -o /out/camera-switch cmd/camera-switch/main.go && \
  mkdir -p /out/lib && \
  cp -L /opt/objectbox/lib/libobjectbox.so /out/lib/

FROM scratch AS artifact
COPY --from=build /out /

## Build with the following command
# docker build --platform "linux/arm/v6" --output "./bin"  .
