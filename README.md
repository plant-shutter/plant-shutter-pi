# plant-shutter for raspberry pi


## Storage

```
.
└── root/
    ├── <project-name>/
    │   ├── <image-name>
    │   ├── ...
    │   └── latest.json
    ├── ...
    └── project.json
```


## Build

```sh
docker build --platform "linux/arm/v6" --output "./bin"  .
```

## Driver

v4l2

> https://github.com/vladimirvivien/go4vl

```shell
sudo raspi-config
```

## Other

### icon

> https://favicon.io/emoji-favicons/blossom/

### PixelViewer

> https://carinastudio.azurewebsites.net/PixelViewer/


### Imager utils

> https://github.com/disintegration/imaging
> https://gist.github.com/logrusorgru/570d64fd6a051e0441014387b89286ca
> https://github.com/nfnt/resize
> https://github.com/icza/mjpeg