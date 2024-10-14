#!/bin/sh
# Run the network
python3 create-network.py
docker compose -f docker-network.yaml down
#delete sn image
cd StorageNode
docker image rm sto:1.0
go build .
docker build -t sto:1.0 .
cd ..
#delete gw image
cd LSDI_Gateway
docker image rm gw:1.0
go build .
docker build -t gw:1.0 .
cd ..
#delete DiscoveryNode image
cd DiscoveryNode
docker image rm disc:1.0
go build .
docker build -t disc:1.0 .
cd ..
# #delete CSS image
# cd CSS
# cd mysql-docker
# docker image rm mysql:1.0
# docker build -t mysql:1.0 .
# cd ..
# docker image rm css:1.0
# go build .
# docker build -t css:1.0 .
# cd ..
# #delete IOT app image
# cd IOT_App
# docker image rm iot:1.0
# go build .
# docker build -t iot:1.0 .
# cd ..
#up network
COMPOSE_PARALLEL_LIMIT=1000 docker compose -f docker-network.yaml up --remove-orphans