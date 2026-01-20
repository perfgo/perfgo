
IMAGE_NAME ?= simonswine/perf-image

push:
	docker buildx build --platform linux/amd64,linux/arm64 --push -t $(IMAGE_NAME) .

build:
	docker build -t $(IMAGE_NAME) .

