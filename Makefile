build:
	docker build --rm -t spiderhouse:latest .

run:
	docker run --env-file .env --publish 8080:8080 --name spiderhouse --rm spiderhouse

clean:
	docker image prune -f

exec:
	#docker exec -it --user root:root spiderhouse /bin/sh
	docker exec -it spiderhouse /bin/sh

# Test commands for cron jobs in Alpine
# run-parts /etc/periodic/15min/