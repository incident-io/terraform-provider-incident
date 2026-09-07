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
#
# The suite has grown past `go test`'s ten minute default timeout, so set one
# explicitly: without it the test binary aborts the whole run mid-test, whatever
# budget the caller thought it had given it. Keep this below the workflow's own
# job timeout, so a genuinely stuck test panics with a stack trace naming itself
# rather than being killed by the runner with nothing to show for it.
#
# TESTARGS comes last, so passing your own -timeout still wins.
.PHONY: testacc
testacc:
	TF_ACC=1 go test ./internal/provider -v -timeout 18m $(TESTARGS)

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
