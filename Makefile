dockbuild:
	docker build --rm -t spiderhouse:latest .

dockrun:
	docker run --publish 8080:8080 --name spiderhouse --rm spiderhouse
