default: testacc

################################################################################
# Development
################################################################################

# Run acceptance tests
.PHONY: testacc
testacc:
	TF_ACC=1 go test ./... -v $(TESTARGS) -timeout 2m

.PHONY: debug
debug:
	TF_ACC=1 dlv test ./internal/provider -v $(TESTARGS) -timeout 2m

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
