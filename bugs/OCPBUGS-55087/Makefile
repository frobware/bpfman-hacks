# https://quay.io/repository/redhat-user-workloads/ocp-bpfman-tenant/ocp-bpfman-operator-catalog-ocp4-19?tab=tags

CATALOG_IMAGE_WITH_DIGEST := $(shell grep "image:" bpfman-catalogsource.yaml | awk '{print $$2}')

render-catalog:
	opm render $(CATALOG_IMAGE_WITH_DIGEST) --output=yaml > bpfman-catalog.yaml

verify:
	@echo CATALOG_IMAGE_WITH_DIGEST=$(CATALOG_IMAGE_WITH_DIGEST)

.PHONY: render-catalog verify
