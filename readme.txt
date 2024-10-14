down network:
docker-compose -f docker-network.yaml down

for gateway
Delete image: docker image rm gw:1.0 (if present)
GO build :go build .
Build image: docker build -t gw:1.0 .

for storage node
Delete image: docker image rm sto:1.0 (if present)
GO build :go build .
Build image: docker build -t sto:1.0 .

for discovery node
Delete image: docker image rm disc:1.0 (if present)
GO build :go build .
Build image: docker build -t disc:1.0 .

After creating images:
up network:
docker-compose -f docker-network.yaml up