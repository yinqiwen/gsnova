ARCH := $(shell sh -c 'arch 2>/dev/null || echo not')
ifeq ($(uname_S),x86_64)
   APPENGINE_PKG=$(GOAPPENGINEROOT)/goroot/pkg/linux_amd64
else
   APPENGINE_PKG=$(GOAPPENGINEROOT)/goroot/pkg/linux_386
endif

