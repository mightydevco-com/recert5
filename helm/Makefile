all: clean test format build

build:
	./build.sh

test: helmlint charttest helmunittest

helmlint:
	./helmlint.sh

charttest:
	./chart-test.sh

helmunittest:
	./helm-unittest.sh

format:
	echo "No format required for $$PWD"

clean:
	rm -rf archives/*.tgz
