PLUGIN_NAME := cpa-models-speed-test
BIN_DIR := bin

UNAME_S := $(shell uname -s)
ifeq ($(OS),Windows_NT)
PLUGIN_EXT := dll
else ifeq ($(UNAME_S),Darwin)
PLUGIN_EXT := dylib
else
PLUGIN_EXT := so
endif

OUTPUT := $(BIN_DIR)/$(PLUGIN_NAME).$(PLUGIN_EXT)

.PHONY: all build build-web test clean

all: build-web build

build-web:
	@echo "Building frontend with Bun..."
	@cd web && bun install && bun run build

build: $(OUTPUT)

$(BIN_DIR):
	@mkdir -p $(BIN_DIR)

$(OUTPUT): main.go web/dist/index.html | $(BIN_DIR)
	@echo "Building CGO plugin library $(OUTPUT)..."
	go build -buildmode=c-shared -o $(OUTPUT) .
	@rm -f $(BIN_DIR)/$(PLUGIN_NAME).h

test:
	go test -v ./...

clean:
	rm -rf $(BIN_DIR) web/dist
