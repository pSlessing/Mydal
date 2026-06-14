BINARY=server
MAIN=./src/cmd/server/
SWAG_DIR=src
SWAG_CMD=swag init --generalInfo cmd/server/main.go --output cmd/server/docs --parseDependency --parseInternal

.PHONY: all swagger build run clean

all: swagger build

swagger:
	cd $(SWAG_DIR) && $(SWAG_CMD)

build:
	go build -o $(BINARY) $(MAIN)

run: build
	./$(BINARY)

clean:
	rm -f $(BINARY)
	rm -rf $(SWAG_DIR)/cmd/server/docs/
