set positional-arguments

build:
    go build -o wo ./cmd/wo
    go build -o wod ./cmd/wod

install: build
    install -m 755 wo wod /usr/local/bin

test:
    go test ./...

lint:
    go vet ./...

fmt:
    go fmt ./...

clean:
    rm -f wo wod

check: lint test

all: build
