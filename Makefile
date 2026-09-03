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

# How many acceptance tests may run at once. Go defaults this to GOMAXPROCS, which on a CI
# runner is a property of the machine rather than of what the suite is waiting for, so set
# it explicitly.
#
# The tests are almost entirely waiting on the API, and every CI leg tests a different
# Terraform CLI against the same organisation with the same credentials. So what this really
# wants to be is however much concurrency that organisation is happy to serve, which is well
# below anything the runner would pick. The tests themselves don't care what it's set to.
TESTACC_PARALLEL ?= 2

# Run acceptance tests
.PHONY: testacc
testacc:
	TF_ACC=1 go test ./internal/provider -v -parallel $(TESTACC_PARALLEL) $(TESTARGS)

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
