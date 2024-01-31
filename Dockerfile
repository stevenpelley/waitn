FROM golang:bullseye
RUN apt update
RUN apt install git
WORKDIR /workspace
RUN git clone https://github.com/stevenpelley/waitn.git
ENTRYPOINT ["go", "build", "-C", "/workspace/waitn/", "./cmd/waitn"]