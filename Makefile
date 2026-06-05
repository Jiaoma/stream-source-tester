GO ?= go

.PHONY: test test-rtsp test-rtp test-mutation build fmt

test:
	$(GO) test ./...

test-rtsp:
	$(GO) test ./internal/output/builtin/rtsp/...

test-rtp:
	$(GO) test ./internal/output/builtin/rtpudp/...

test-mutation:
	$(GO) test ./internal/mutation/... ./internal/output/builtin/rtpudp/... -run 'Mutation|mutat'

build:
	$(GO) build ./...

fmt:
	find . -name '*.go' -print0 | xargs -0 gofmt -w
