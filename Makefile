build:
	docker build --rm -t spiderhouse:latest .

# Warning you cannot detach process with ctrl-c becasue --rm is used. Must use Ctrl+p followes by Ctrl+q
# https://stackoverflow.com/questions/19688314/how-do-you-attach-and-detach-from-dockers-process
run:
	docker run -t -i --rm  --env-file .env --publish 8080:8080 --name spiderhouse --rm spiderhouse

clean:
	docker image prune -f

exec:
	docker exec -it spiderhouse /bin/sh

execroot:
	docker exec -it --user root:root spiderhouse /bin/sh


