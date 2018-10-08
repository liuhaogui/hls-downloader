BINARY=hls-downloader

VERSION=`git describe --tags`
BUILD=`date +%FT%T%z`

LDFLAGS=-ldflags "-w -s -X main.Version=${VERSION} -X main.Build=${BUILD}"

build:
	go build ${LDFLAGS} -o ${BINARY}

install:
	go install ${LDFLAGS}

clean:
	if [ -f ${BINARY} ] ; then rm ${BINARY} ; fi

.PHONY: clean install