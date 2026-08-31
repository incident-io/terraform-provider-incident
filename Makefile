default: testacc

################################################################################
# Development
################################################################################

# Run the unit tests: every package, with no API credentials needed. The
# acceptance tests are gated behind TF_ACC and skip themselves without it, so
# this covers the pure-Go suites (models, jsontypes, richtexttypes, apischema)
# that `testacc` never reaches.
.PHONY: test
test:
	go test ./... $(TESTARGS)

# Run acceptance tests
.PHONY: testacc
testacc:
	TF_ACC=1 go test ./internal/provider -v $(TESTARGS)

.PHONY: debug
debug:
	TF_ACC=1 dlv test ./internal/provider -v $(TESTARGS)

# Installs tools as defined in tools/tools.go
.PHONY: install
install:
	go install

.PHONY: build
build:
	go build -o bin/terraform-provider-incident .

.PHONY: generate
generate:
	go generate ./...
