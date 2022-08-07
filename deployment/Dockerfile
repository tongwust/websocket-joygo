# 启动编译环境
#FROM golang:1.15-alpine AS builder
FROM golang:1.17.1-alpine3.14 AS builder

# 配置编译环境
RUN go env -w GO111MODULE=on
RUN go env -w GOPROXY=https://goproxy.cn,direct

# 拷贝源代码到镜像中
COPY . /go/src/joygosrv

# 编译
WORKDIR /go/src/joygosrv
RUN go install ./...

#FROM alpine:3.13
FROM alpine:3.14
COPY --from=builder /go/bin/joygosrv /bin/joygosrv

ENV ADDR=:8090
ENV AMQP_URL=amqp://jtest:123qwe@172.25.27.86:5672/
ENV CERT_FILE=/www/fullchain.pem
ENV KEY_FILE=/www/privkey.pem
ENV ISTEST=0

# 申明暴露的端口
EXPOSE 8090

# 设置服务入口
ENTRYPOINT [ "/bin/joygosrv" ]