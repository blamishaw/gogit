build:
	go build .

install:
	make build && chmod +x gogit
	sudo cp gogit /usr/local/bin

test:
	mkdir /tmp/gogit_test ; go test ./... ; rm -rf /tmp/gogit_test

reset:
	rm -rf .gogit

copy-graph:
	go run . k | pbcopy