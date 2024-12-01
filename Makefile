build:
	CGO_ENABLED=0 GOARCH=amd64 GOOS=linux go build -a -tags netgo -ldflags '-w -extldflags -static' -o bin/go-http-file-server-linux-amd64 .
