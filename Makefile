.PHONY: all build clean run check cover lint docker help
BIN_FILE=onenode
all: check build
build:
	@go build -o "${BIN_FILE}" examples/onenode/node.go
clean:
	@go clean
	rm --force "xx.out"
test:
	@go test
check:
	@go fmt ./
	@go vet ./
cover:
	@go test -coverprofile xx.out
	@go tool cover -html=xx.out
run:
	./"${BIN_FILE}"
lint:
	golangci-lint run
docker:
	@docker build -t leo/hello:latest .
help:
	@echo "make format - Formats the Go code and compiles it into a binary file."
	@echo "make build - Compiles Go code into a binary file."
	@echo "make clean - Cleans up intermediate target files."
	@echo "make test - Executes the test cases."
	@echo "make check - Formats the Go code."
	@echo "make cover - Checks the test coverage."
	@echo "make run - Runs the program directly."
	@echo "make lint - Performs code linting."
	@echo "make docker - Builds a Docker image."