mac:
	GOOS=darwin GOARCH=amd64 go build -o isolate_mac ../../isolate
	GOOS=darwin GOARCH=amd64 go build -o isolated_mac ../../isolated

linux:
	GOOS=linux GOARCH=amd64 go build ../../isolate
	GOOS=linux GOARCH=amd64 go build ../../isolated

win:
	GOOS=windows GOARCH=amd64 go build ../../isolate
	GOOS=windows GOARCH=amd64 go build ../../isolated
