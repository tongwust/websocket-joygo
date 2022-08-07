DOMAIN=$1
cd ../deployment
docker build -t kucar/$DOMAIN -f ../deployment/$DOMAIN/Dockerfile .

#docker build -t joygosrv:v1.1 -f ./deployment/Dockerfile .
#docker run --name joygosrv -d -p 8090:8090 joygosrv:v1.1
#docker run -d --name joygosrv -p 8090:8090 -v /tmp/logs/:/tmp/logs/ -v /www/server/panel/vhost/cert/www.joygo77.com/:/www/ joygosrv:v1.1
