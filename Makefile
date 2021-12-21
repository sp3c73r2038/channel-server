CGO_ENABLED := 0
GOOS := linux
REGISTRY := registry.ops.gaoshou.me

export CGO_ENABLED
export GOOS

build:
	go build -o bin/channel-server cmd/channel-server/main.go

clean:
	rm -rf bin/*

docker:
	docker build -t $(REGISTRY)/channel-server .

docker-push:
	docker push $(REGISTRY)/channel-server
