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