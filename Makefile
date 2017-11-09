PROJECT_NAME   = fabric-network-composer
BASE_VERSION   = 0.1.0
BINARY_NAME = netcomposer

PLATFORMS = windows-amd64 darwin-amd64 linux-amd64 linux-ppc64le linux-s390x

.PHONY:	binaries
.PHONY:	clean

# builds bin PACKAGE for all target platforms
binaries: $(patsubst %,binaries/%, $(PLATFORMS))

binaries/windows-amd64:	GOOS=windows
binaries/windows-amd64:	GO_TAGS+= nopkcs11
binaries/windows-amd64:	$(patsubst %,bin/windows-amd64/%, $(BINARY_NAME)) bin/windows-amd64

binaries/darwin-amd64:	GOOS=darwin
binaries/darwin-amd64:	GO_TAGS+= nopkcs11
binaries/darwin-amd64:	$(patsubst %,bin/darwin-amd64/%, $(BINARY_NAME)) bin/darwin-amd64

binaries/linux-amd64:	GOOS=linux
binaries/linux-amd64:	GO_TAGS+= nopkcs11
binaries/linux-amd64:	$(patsubst %,bin/linux-amd64/%, $(BINARY_NAME)) bin/linux-amd64

binaries/%-amd64:	DOCKER_ARCH=x86_64
binaries/%-amd64:	GOARCH=amd64
binaries/linux-%:	GOOS=linux

binaries/linux-ppc64le:	GOARCH=ppc64le
binaries/linux-ppc64le:	DOCKER_ARCH=ppc64le
binaries/linux-ppc64le:	GO_TAGS+= nopkcs11
binaries/linux-ppc64le:	$(patsubst %,bin/linux-ppc64le/%, $(BINARY_NAME)) bin/linux-ppc64le

binaries/linux-s390x:	GOARCH=s390x
binaries/linux-s390x:	DOCKER_ARCH=s390x
binaries/linux-s390x:	GO_TAGS+= nopkcs11
binaries/linux-s390x:	$(patsubst %,bin/linux-s390x/%, $(BINARY_NAME)) bin/linux-s390x

bin/%:	$(PROJECT_FILES)
	@echo "Building $(BINARY_NAME) for $(GOOS)-$(GOARCH)"
	mkdir -p $(@D)
	$(CGO_FLAGS) GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(abspath $@) -tags "$(GO_TAGS)" -ldflags "$(GO_LDFLAGS)"
	rsync -rupE sample-chaincodes bin/$(GOOS)-$(GOARCH)
	cp samplenet.yaml bin/$(GOOS)-$(GOARCH)/samplenet.yaml
	@echo "Building tools for $(GOOS)-$(GOARCH)"
	$(CGO_FLAGS) GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o ./tools/$(GOOS)-$(GOARCH)/configtxgen -tags "$(GO_TAGS)" -ldflags "$(GO_LDFLAGS)" github.com/hyperledger/fabric/common/configtx/tool/configtxgen
	$(CGO_FLAGS) GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o ./tools/$(GOOS)-$(GOARCH)/cryptogen -tags "$(GO_TAGS)" -ldflags "$(GO_LDFLAGS)" github.com/hyperledger/fabric/common/tools/cryptogen
	rsync -rupE templates/ bin/$(GOOS)-$(GOARCH)/templates
	rsync -rupE tools/$(GOOS)-$(GOARCH) bin/$(GOOS)-$(GOARCH)/tools
	cd $(@D) && tar czf ../../bin/netcomposer-$(GOOS)-$(GOARCH).tar.gz .

clean:
	@rm -rf bin
