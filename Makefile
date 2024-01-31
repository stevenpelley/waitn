build:
	docker build . --file ./Dockerfile -t waitnbuildimage:1
	docker run --name waitnbuildcontainer waitnbuildimage:1
	docker cp waitnbuildcontainer:/workspace/waitn/waitn .
	docker container rm "`docker container ls -f "name=waitnbuildcontainer" -a -q`"
	docker image rm "`docker image ls -q waitnbuildimage:1`"
