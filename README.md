# plant-shutter for raspberry pi


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

## 启用RNDIS

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