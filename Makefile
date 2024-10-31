build:
	go build .

test:
	mkdir /tmp/gogit_test ; go test ./... ; rm -rf /tmp/gogit_test

reset:
	rm -rf .gogit/*

copy-graph:
	go run . k | pbcopy