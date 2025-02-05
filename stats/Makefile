BPF_CLANG_FLAGS ?= -g -O2 -Wall -target bpf
BPF_DIR         ?= bpf
BPF_SRCS        := $(wildcard $(BPF_DIR)/*.c)
BPF_OBJS        := $(patsubst %.c,%.o,$(BPF_SRCS))

build: $(BPF_OBJS)

$(BPF_DIR)/%.o: $(BPF_DIR)/%.c $(BPF_DIR)/vmlinux.h Makefile
	$(COMPILE.c) $(BPF_CLANG_FLAGS) -o $@ $<

$(BPF_DIR)/vmlinux.h:
	bpftool btf dump file /sys/kernel/btf/vmlinux format c > $@

clean:
	rm -f $(BPF_OBJS) $(BPF_DIR)/vmlinux.h

.PHONY: build clean
