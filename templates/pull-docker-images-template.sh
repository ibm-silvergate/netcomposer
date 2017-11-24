#!/bin/bash -eu

# This script pulls docker images from the Dockerhub hyperledger repositories

# set the default Docker namespace and tag
DOCKER_NS={{.DockerNS}}
ARCH={{.Arch}}
VERSION={{.Version}}

# set of Hyperledger Fabric images
FABRIC_IMAGES=(fabric-peer fabric-orderer fabric-ccenv fabric-javaenv fabric-kafka fabric-zookeeper fabric-couchdb)

for image in ${FABRIC_IMAGES[@]}; do
	echo "Pulling ${DOCKER_NS}/$image:${ARCH}-${VERSION}"
	docker pull ${DOCKER_NS}/$image:${ARCH}-${VERSION}
done
