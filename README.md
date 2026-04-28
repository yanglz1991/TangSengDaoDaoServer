## QX

`开发环境需要go >=1.20`


启动基础环境镜像 docker 启动 MySQL Redis wukongim

```shell
docker compose -f testenv/docker-compose.yaml up -d
```

运行代码 先修改服务ip `testenv/docker-compose.yaml` 中的 `WK_EXTERNAL_IP`

```shell
go run main.go
```

打包镜像推送阿里云

```shell
make deploy
```

修改默认配置 `configs/tsdd.yaml`

https

把本地 nginx 目录整个上传到服务器
本地要上传的文件（4 个）：

docker/tsdd/nginx/nginx.conf
docker/tsdd/nginx/conf.d/tsdd.conf
docker/tsdd/nginx/certs/qx_qhfhasina_com.crt
docker/tsdd/nginx/certs/qx_qhfhasina_com.key

上传到服务器的：

/www/server/panel/data/compose/qx-1/nginx/nginx.conf
/www/server/panel/data/compose/qx-1/nginx/conf.d/tsdd.conf
/www/server/panel/data/compose/qx-1/nginx/certs/qx_qhfhasina_com.crt
/www/server/panel/data/compose/qx-1/nginx/certs/qx_qhfhasina_com.key


```shell
rsync -avz docker/tsdd/nginx/ root@47.239.98.68:/www/server/panel/data/compose/qx-2/nginx/
```

使用 https 之后

管理端
https://qx.qhfhasina.com:8443/login

客户端
https://qx.qhfhasina.com