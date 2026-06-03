.PHONY: test build swagger

test:
	go test -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | grep total:
	@pct=$$(go tool cover -func=coverage.out | grep total: | awk '{print $$3}' | tr -d '%'); \
	  threshold=80; \
	  if [ "$$(echo "$$pct < $$threshold" | awk '{print ($$1 < $$3)}')" = "1" ]; then \
	    echo "FAIL: coverage $$pct% is below $$threshold%"; exit 1; \
	  else \
	    echo "PASS: coverage $$pct% meets $$threshold% threshold"; \
	  fi

build:
	CGO_ENABLED=0 GOOS=linux go build -o moviesx ./cmd/moviesx

swagger:
	go run github.com/swaggo/swag/cmd/swag init \
		-g cmd/moviesx/main.go \
		--parseDependency --parseInternal \
		-o docs
