
VERSION = 5.0.0
documentation:
	cd docs && hugo -D -b recert5
	rm -rf ../websites/uberscott.com/html/recert5
	mkdir ../websites/uberscott.com/html/recert5
	cp -r docs/public/* ../websites/uberscott.com/html/recert5

clean: 
	echo "HELLO"
	cd docs && rm -rf public 

docker-build: 
	cd docker/certbot && docker build . --tag uberscott/recert5-certbot:$(VERSION)-snapshot 
	docker push uberscott/recert5-certbot:$(VERSION)-snapshot
	cd docker/nginx && docker build . --tag uberscott/recert5-nginx:$(VERSION)-snapshot 
	docker push uberscott/recert5-nginx:$(VERSION)-snapshot

all: docker


