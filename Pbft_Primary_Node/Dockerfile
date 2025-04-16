FROM ubuntu:latest

RUN apt-get update && rm -rf /var/lib/apt/lists/*

COPY ./Pbft_Primary_Node ./

CMD ["./Pbft_Primary_Node"]