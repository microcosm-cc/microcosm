# Targets:
#
#   all:          Builds the code locally after testing
#
#   fmt:          Formats the source files
#   build:        Builds the code locally
#   vet:          Vets the code
#   test:         Runs the tests
#   clean:        Deletes the locally built file (if it exists)
#
#   dep_restore:  Ensures all dependent packages are at the correct version
#   dep_update:   Ensures all dependent packages are at the latest version

export GO15VENDOREXPERIMENT=1


.PHONY: all fmt build vet test clean dep_restore dep_update

# The first target is always the default action if `make` is called without args
all: clean vet test build

build: export GOOS=linux
build: export GOARCH=amd64
build: clean
	@go build cmd/microcosm/microcosm.go

vet:
	@go vet $$(go list ./... | grep -v /vendor/)

test:
	@go test $$(go list ./... | grep -v /vendor/)

clean:
	@find . -maxdepth 1 -name microcosm -delete

fetch:
	-gvt fetch github.com/bradfitz/gomemcache/memcache
	-gvt fetch github.com/cloudflare/ahocorasick
	-gvt fetch github.com/disintegration/imaging
	-gvt fetch github.com/golang/glog
	-gvt fetch github.com/gorilla/mux
	-gvt fetch github.com/lib/pq
	-gvt fetch github.com/microcosm-cc/bluemonday
	-gvt fetch github.com/microcosm-cc/goconfig
	-gvt fetch github.com/microcosm-cc/exifutil
	-gvt fetch github.com/mitchellh/goamz/aws
	-gvt fetch github.com/mitchellh/goamz/s3
	-gvt fetch github.com/nytimes/gziphandler
	-gvt fetch github.com/robfig/cron
	-gvt fetch github.com/russross/blackfriday
	-gvt fetch github.com/rwcarlsen/goexif/exif
	-gvt fetch github.com/tools/godep
	-gvt fetch github.com/xtgo/uuid
	-gvt fetch golang.org/x/net/html
	-gvt fetch golang.org/x/oauth2
