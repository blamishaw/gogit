build:
	go build .

install:
	make build && chmod +x gogit
	sudo cp gogit /usr/local/bin

# Can set PERSIST=true when calling to debug test files
test:
	mkdir /tmp/gogit_test && (go test -v ./... || true)
ifneq ("$(PERSIST)", "true")
	rm -rf /tmp/gogit_test
endif

reset:
	rm -rf .gogit

copy-graph:
	go run . k | pbcopy