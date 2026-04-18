deploy:
	docker build -t tangsengdaodaoserver . --platform linux/arm64/v8
	docker login --username=hi50071365@aliyun.com crpi-10spfkgd32nbn5ev.cn-shanghai.personal.cr.aliyuncs.com
	docker tag tangsengdaodaoserver crpi-10spfkgd32nbn5ev.cn-shanghai.personal.cr.aliyuncs.com/qxim/server:latest
	docker push crpi-10spfkgd32nbn5ev.cn-shanghai.personal.cr.aliyuncs.com/qxim/server:latest

run-dev:
	docker-compose build;docker-compose up -d
stop-dev:
	docker-compose stop
env-test:
	docker-compose -f ./testenv/docker-compose.yaml up -d 