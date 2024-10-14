FROM ubuntu:latest

COPY ./LSDI_Gateway ./
COPY ./image.png ./pics/
COPY ./index.html ./

RUN apt-get update && rm -rf /var/lib/apt/lists/*


RUN apt-get update && apt-get install -y \
  graphviz \
  && rm -rf /var/lib/apt/lists/*

CMD ["./LSDI_Gateway"]