build_local:
	go build -o bin/plant-shutter main.go

build_test:
	go build -o bin/plant-shutter cmd/test/main.go