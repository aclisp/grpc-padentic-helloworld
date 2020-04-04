
all: test_go

test_go: helloworld.proto
	PATH="${GOPATH}/bin:${PATH}" protoc \
	  -I. \
		-I${GOPATH}/src \
		--go_out=plugins=grpc:. \
		helloworld.proto


