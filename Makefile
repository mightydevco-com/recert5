dox:
	cd docs && hugo -D -b recert5
	rm -rf ../websites/uberscott.com/html/recert5
	mkdir ../websites/uberscott.com/html/recert5
	cp -r docs/public/* ../websites/uberscott.com/html/recert5

clean: 
	echo "HELLO"
	cd docs && rm -rf public 


all: clean dox


