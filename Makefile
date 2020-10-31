dokbuild:
	docker build --rm -t spiderhouse:latest .

dokrun:
	docker run --publish 8080:8080 --name spiderhouse --rm spiderhouse

dokclean:
	docker image prune -f

dokexec:
	docker exec -it --user root:root spiderhouse /bin/sh