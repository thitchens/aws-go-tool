.PHONY: all build upload clean

all: build upload

build:
	@echo "Building linux binary"
	env GOOS=linux go build ./
	zip aws-go-tool-linux.zip aws-go-tool
	@echo "Building windows binary"
	env GOOS=windows go build ./
	zip aws-go-tool-windows.zip aws-go-tool.exe
	@echo "Building mac binary"
	env GOOS=darwin go build ./
	zip aws-go-tool-mac.zip aws-go-tool
	
upload:
	@echo "Uploading zip files"
	aws s3 cp ./aws-go-tool-linux.zip s3://afeeblechild/go-binaries/aws-go-tool-linux.zip --profile personal-upload
	aws s3 cp ./aws-go-tool-windows.zip s3://afeeblechild/go-binaries/aws-go-tool-windows.zip --profile personal-upload
	aws s3 cp ./aws-go-tool-mac.zip s3://afeeblechild/go-binaries/aws-go-tool-mac.zip --profile personal-upload

clean:
	@echo "Removing zip files"
	rm *.zip
	rm aws-go-tool.exe
	rm aws-go-tool
