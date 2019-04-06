TAG?=latest

build:
	docker build -t appsbyram/seafoodtruck-slack:$(TAG) .

push:
	docker push appsbyram/seafoodtruck-slack:$(TAG) .
	

