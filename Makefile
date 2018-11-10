deps:
	@echo "--> Downloading packr..."
	@go get -u github.com/gobuffalo/packr/packr
	@echo "--> Installing packr..."
	@go install "${GOPATH}/src/github.com/gobuffalo/packr/packr"
	@echo "--> Running dep ensure..."
	@dep ensure -v

clean:
	packr clean
	rm -rf ./target

packr:
	packr

build: packr
	CGO_ENABLED=1 go build -o ./target/chaind ./cmd/chaind/main.go

install-global: build
	sudo mv ./target/chaind /usr/bin

.PHONY: build