
all: test_go

test_go: router.proto
	PATH="${GOPATH}/bin:${PATH}" protoc \
	  -I. \
		-I${GOPATH}/src \
		--go_out=plugins=grpc:. \
		router.proto


