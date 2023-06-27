# plant-shutter for raspberry pi

![icon](asset/icon-192x192.png)

一个简单易用的延时摄影（TimeLapse）程序。

> 成片B站视频

***

## Features

* 支持生成**预览视频**
* **实时预览**调参
* 支持使用**WebDAV**共享拍摄的图片
* 使用`Video for Linux 2` (**v4l2**) API
* **All-In-One**，开箱即用

## QuickStart

```sh
chmod +x plant-shutter
./plant-shutter
```

在浏览器打开[管理界面](raspberry:9999)

## Systemd



## Storage

```
.
└── root/
    ├── <project-name>/
    │   ├── images/
    │   │   ├── <image>.jpg
    │   │   ├── ...
    │   │   └── info.json
    │   └── videos/
    │       ├── <name>.avi
    │       └── ...
    ├── ...
    └── info.json
```


## Build

```sh
docker build --platform "linux/arm/v6" --output "./bin"  .
```

## RNDIS

树莓派的网络性能有限，如果你使用的zero w，那文件传输速率仅有2-3MB/s，使用RNDIS，将树莓派通过usb连接电脑，并将树莓派识别成网络设备，可以直接通过ip:port的方式访问树莓派，提升文件下载速度。

> https://learn.adafruit.com/turning-your-raspberry-pi-zero-into-a-usb-gadget/ethernet-gadget

```bash
vi /boot/config.txt
# 最后一行新增
dtoverlay=dwc2

vi /etc/modules
# rootwait后添加空格和如下内容
modules-load=dwc2,g_ether
```

安装RNDIS驱动

通过usb连接树莓派

## Hardware

测试硬件

* [raspberry pi zero w](https://www.raspberrypi.com/products/raspberry-pi-zero-w/)
* [pi camera(module v2)](https://www.raspberrypi.com/products/camera-module-v2/)

## Todo

- [x] 图片存储结构
- [x] 堆叠视频
- [x] 相机管理
- [x] 相机参数调节
- [x] 任务调度与状态管理
- [x] 测试RNDIS

## Driver

v4l2

> https://github.com/vladimirvivien/go4vl

```shell
# enable driver
sudo raspi-config
```

## Other

### icon

> https://favicon.io/emoji-favicons/blossom/

### PixelViewer

> https://carinastudio.azurewebsites.net/PixelViewer/


### Image utils

> https://github.com/disintegration/imaging
> https://gist.github.com/logrusorgru/570d64fd6a051e0441014387b89286ca
> https://github.com/nfnt/resize
> https://github.com/icza/mjpeg

### pi camera

> https://www.raspberrypi.com/documentation/accessories/camera.html
