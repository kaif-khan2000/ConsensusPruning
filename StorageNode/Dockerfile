FROM ubuntu:latest

COPY ./StorageNode ./
COPY ./image.png ./pics/
COPY ./index.html ./

RUN apt-get update && apt-get install -y \
  graphviz \
  && rm -rf /var/lib/apt/lists/*

CMD ["./StorageNode"]