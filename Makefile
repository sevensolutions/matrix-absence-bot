.PHONY: build run vet tidy docker

# goolm selects mautrix-go's pure-Go olm/megolm implementation instead of
# cgo bindings to system libolm. Required for every build/run/vet invocation.
TAGS := -tags goolm

IMAGE := matrix-absence-bot

build:
	go build $(TAGS) -o matrix-absence-bot .

run:
	go run $(TAGS) .

vet:
	go vet $(TAGS) ./...

tidy:
	go mod tidy

docker:
	docker build -t $(IMAGE) .
