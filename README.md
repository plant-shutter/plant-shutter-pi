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

## Todo

- [x] 图片存储结构
- [ ] 堆叠视频
- [ ] 相机管理
- [ ] 相机参数调节
- [ ] 任务调度与状态管理

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