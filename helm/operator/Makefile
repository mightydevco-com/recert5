all: tar

crds:
	cd ../../go/src/operator && $(MAKE) crds
	../../tool/cp-crd-to-helm.sh

tar: crds
	cd chart && tar czvf ../../../out/operator.tgz .
