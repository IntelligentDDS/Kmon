RELEASE_PATH ?= ./release

.PHONY: clean

all: monitor obj

test: monitor
	cd release && ./kmon

monitor:
	go build -o $(RELEASE_PATH)/kmon src/main.go

obj:
	make -C ebpf all

docker:
	docker build -t kmon:latest .

clean:
	rm $(RELEASE_PATH)/kmon
	make -C ebpf clean