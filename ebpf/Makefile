CC = gcc
CLANG = clang-9
LLC = llc-9

ROOT_DIR ?= .
RELEASE_PATH ?= ../release/libbpf
LIBBPF_ROOT_DIR ?= $(ROOT_DIR)/../libbpf

LINUX_PATH = /usr/src/linux-headers-5.4.0-050400-generic
LIBBPF_PATH = ../libbpf/src/libbpf.a

BPF_INCLUDE_FLAGS = -I$(LINUX_PATH)/arch/x86/include
BPF_INCLUDE_FLAGS += -I$(LINUX_PATH)/arch/x86/include/generated/uapi
BPF_INCLUDE_FLAGS += -I$(LINUX_PATH)/arch/x86/include/generated
BPF_INCLUDE_FLAGS += -I$(LINUX_PATH)/include
# linux/include必须在 libbpf前面
BPF_INCLUDE_FLAGS += -I$(LIBBPF_ROOT_DIR)/src/build/usr/include/
BPF_INCLUDE_FLAGS += -I$(LIBBPF_ROOT_DIR)/include/uapi
BPF_INCLUDE_FLAGS += -I$(LINUX_PATH)/arch/x86/include/uapi
BPF_INCLUDE_FLAGS += -I$(LINUX_PATH)/arch/x86/include/generated/uapi
BPF_INCLUDE_FLAGS += -I$(LINUX_PATH)/include/uapi
BPF_INCLUDE_FLAGS += -I$(LINUX_PATH)/include/generated/uapi

CFLAGS ?= -g -DLOG_USE_COLOR
BPF_CFLAGS = -include $(LINUX_PATH)/include/linux/kconfig.h
# hack 的头文件
BPF_CFLAGS += -include $(ROOT_DIR)/include/hack.h
LIBS = -L./ -lbpf -lelf

.PHONY: all clean


ALL_OBJ = $(wildcard ./src/*.c)
all: $(ALL_OBJ:.c=.o)

%.o: %.c
	@$(CLANG) \
		-D __BPF_TRACING__ -D__TARGET_ARCH_x86 -D __KERNEL__ \
		-D __HAVE_BUILTIN_BSWAP16__ -D __HAVE_BUILTIN_BSWAP32__ \
		-D __HAVE_BUILTIN_BSWAP64__ \
		-nostdinc -isystem $(shell $(CLANG) -print-file-name=include) \
		$(BPF_INCLUDE_FLAGS) \
		$(BPF_CFLAGS) \
		-Wno-deprecated-declarations \
		-Wno-gnu-variable-sized-type-not-at-end \
		-Wno-pragma-once-outside-header \
		-Wno-address-of-packed-member -Wno-unknown-warning-option \
		-Wno-unused-value -Wno-pointer-sign \
		-fdiagnostics-show-option \
		-O2 -emit-llvm -c -g $< \
		-o - | $(LLC) -march=bpf -filetype=obj -o $(RELEASE_PATH)/$(notdir $@)

clean:
	rm $(RELEASE_PATH)/*.o
