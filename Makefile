.PHONY: all test vet fmt-check lint secrets packages clean

all: test packages

test:
	go test ./...

vet:
	go vet ./...

fmt-check:
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }

secrets:
	bash scripts/check-secrets.sh

# needs shellcheck
lint:
	shellcheck -S warning packaging/common/filedrop-setup packaging/common/filedrop-keygen \
		packaging/common/filedrop-desktop-link.sh packaging/common/filedrop-cleanup.sh \
		packaging/tar/*.sh packaging/directory/install.sh packaging/deb/postinst \
		packaging/deb/prerm packaging/deb/postrm scripts/*.sh

# .deb / .rpm / .tar.gz for amd64 and arm64 into ./dist
packages:
	bash scripts/build.sh

clean:
	rm -rf dist
