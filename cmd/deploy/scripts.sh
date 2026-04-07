
docker run -d --name emqx -p 1883:1883 -p 8083:8083 -p 8883:8883 -p 18083:18083 emqx/emqx:latest




# 1. 准备目录并设置正确权限（MySQL 容器内 mysql 用户 uid=999）
sudo mkdir -p /mydata/mysql/{data,log,conf}
sudo chown -R 999:999 /mydata/mysql/data /mydata/mysql/log
# 配置文件目录保持 root 权限即可，或设置为 755

# 2. 如果宿主机 3306 端口已被占用，换一个映射端口（例如 3307:3306）

# 3. 运行容器（修正挂载点）
docker run \
--name mysql \
-d \
-p 3306:3306 \
--restart=unless-stopped \
-v /mydata/mysql/log:/var/log/mysql \
-v /mydata/mysql/data:/var/lib/mysql \
-v /mydata/mysql/conf:/etc/mysql/conf.d \
-e MYSQL_ROOT_PASSWORD=123456 \
mysql:5.7