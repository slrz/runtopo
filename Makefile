GO          ?= go
GOFMT       ?= gofmt
GOLINT      ?= golint
STATICCHECK ?= staticcheck
AOUT        ?= runtopo

# Build runtopo
.PHONY: build
build:
	$(GO) build -o $(AOUT)

# Execute tests
.PHONY: test
test:
	$(GO) test ./...

# Execute tests with race detector instrumentation
.PHONY: race
race:
	$(GO) test -race ./...

# Execute tests, writing coverage profile to coverage/cover.out
.PHONY: cover
cover:
	./tools/cover.sh cover

# Writes coverage html to coverage/cover.html
.PHONY: coverhtml
coverhtml: cover
	./tools/cover.sh coverhtml

# Writes cobertura coverage.xml to coverage/coverage.xml
.PHONY: coberturaxml
coberturaxml: cover
	./tools/cover.sh coberturaxml

# Re-format source code
.PHONY: fmt
fmt:
	$(GO) fmt ./...

# Vet the code
.PHONY: vet
vet:
	$(GO) vet ./...

# Run the staticcheck static analyzer (https://staticcheck.io)
.PHONY: staticcheck
staticcheck:
	$(STATICCHECK) ./...

# Run golint (https://github.com/golang/lint)
.PHONY: lint
lint:
	$(GOLINT) -set_exit_status ./...

.PHONY: clean
clean:
	rm -f $(AOUT)
	rm -rf coverage
