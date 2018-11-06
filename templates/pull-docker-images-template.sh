#!/bin/bash -eu

# This script pulls docker images from the Dockerhub hyperledger repositories

# docker namespace
DOCKER_NS={{.DockerNS}}
# version tag for fabric images (peer, orderer, etc.)
FABRIC_VERSION_TAG={{.FabricVersionTag}}
# version tag for ca image
CA_VERSION_TAG={{.CaVersionTag}}
# version tag for couchdb, kafka, zookeeper
THIRDPARTY_VERSION_TAG={{.ThirdpartyVersionTag}}

pullDockerImage() {
    IMAGE=$1
    echo "Pulling $IMAGE"
    docker pull ${IMAGE}
}

# hyperledger fabric images
FABRIC_IMAGES=(fabric-peer fabric-orderer fabric-ccenv fabric-javaenv)
for image in ${FABRIC_IMAGES[@]}; do
    pullDockerImage ${DOCKER_NS}/${image}:${FABRIC_VERSION_TAG}
done

# ca image
pullDockerImage ${DOCKER_NS}/fabric-ca:${CA_VERSION_TAG}

# thirdparty images
THIRDPARTY_IMAGES=(fabric-zookeeper fabric-couchdb fabric-kafka)
for image in ${THIRDPARTY_IMAGES[@]}; do
    pullDockerImage ${DOCKER_NS}/${image}:${THIRDPARTY_VERSION_TAG}
done







