FROM ubuntu:latest

RUN apt-get update && rm -rf /var/lib/apt/lists/*


RUN apt-get update && apt-get install -y \
  graphviz \
  && rm -rf /var/lib/apt/lists/*

COPY ./LSDI_Gateway ./
COPY ./image.png ./pics/
COPY ./index.html ./

CMD ["./LSDI_Gateway"]