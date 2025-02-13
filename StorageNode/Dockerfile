FROM ubuntu:latest

RUN apt-get update && apt-get install -y \
  graphviz \
  && rm -rf /var/lib/apt/lists/*

COPY ./StorageNode ./
COPY ./image.png ./pics/
COPY ./index.html ./


CMD ["./StorageNode"]