APP     := embedding_benchmark
BUILD   := build
BPE     := cl100k_base.tiktoken
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

# build one target: $(1)=GOOS $(2)=GOARCH $(3)=binary suffix
define build_target
	$(eval DIR := $(BUILD)/$(APP)-$(1)-$(2))
	mkdir -p $(DIR)
	GOOS=$(1) GOARCH=$(2) go build $(LDFLAGS) -o $(DIR)/$(APP)$(3) .
	cp $(BPE) $(DIR)/$(BPE)
endef

.PHONY: all macos linux windows clean

all: macos linux windows

macos:
	$(call build_target,darwin,amd64,)
	$(call build_target,darwin,arm64,)

linux:
	$(call build_target,linux,amd64,)
	$(call build_target,linux,arm64,)

windows:
	$(call build_target,windows,amd64,.exe)
	$(call build_target,windows,arm64,.exe)

clean:
	rm -rf $(BUILD)
