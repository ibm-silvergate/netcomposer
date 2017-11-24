#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

function stopNetwork() {
	echo "Removing containers and chaincode images"

    docker-compose -f ./samplenet/docker-compose.yaml down

	ccContainers=$(docker ps -a  | grep "dev-" | awk '{ print $1 }')
	if [ -z "$ccContainers" ];
	then echo "No chaincode containers found"
	else docker rm $ccContainers
	fi

	ccImages=$(docker images | grep "dev-" | awk '{ print $3 }')
	if [ -z "$ccImages" ];
	then echo "No chaincode images found"
	else docker rmi $ccImages
	fi
}

function startNetwork() {
    docker-compose -f ./samplenet/docker-compose.yaml up -d
}

function createChannel() {
	#$1 peer cli from which the create command is made
	#$2 orderer to which the request is sent
	#$3 channel
    #$4 orderer tls ca certificate

	docker exec $1 /bin/sh -c "cd channel-artifacts; peer channel create -o '$2' -c $3 -f $3.tx -t 10 --tls true --cafile '$4'"
}

function joinPeerToChannel() {
	#$1 peer cli
	#$2 channel

    docker exec $1 /bin/sh -c "cd channel-artifacts; peer channel join -b $2.block"
}

function installChaincode() {
	#$1 peer cli in which the chaincode is installed
	#$2 chaincode name
	#$3 chaincode version
    #$4 chaincode platform (language in which it was coded)
	#$5 chaincode source code path
	docker exec $1 /bin/sh -c "peer chaincode install -n $2 -v $3 -l $4 -p $5"
}

function instantiateChaincode() {
	#$1 peer cli
	#$2 orderer
    #$3 tls enabled
	#$4 orderer tls ca certificate
	#$5 channel
	#$6 chaincode
	#$7 chaincode version
	#$8 data
	#$9 endorsing policy

	docker exec $1 /bin/sh -c "peer chaincode instantiate -o $2 --tls $3 --cafile '$4' -C $5 -n $6 -v '$7' -c '$8' -P '$9'"
}

function panicOnError() {
	if [ $1 -eq 0 ];
	then 
		echo $2
	else
		echo $3
		exit 1
	fi
}

stopNetwork
panicOnError $? "Containers and images successfully cleared!" "Error while stopping current network"

if [ "$1" == "stop" ]; then 
	exit 0
fi

startNetwork
panicOnError $? "Basic containers successfully started!" "Error while starting basic containers (Peers, Orderers, CAs, ...)"

ORDERER_CA='/etc/hyperledger/fabric/crypto-config/orderer/msp/tlscacerts/tlsca.samplenet.com-cert.pem'

createChannel 'cli.peer1.org1.samplenet.com' 'orderer1.samplenet.com:7050' 'bigchannel' $ORDERER_CA
panicOnError $? "Channel 'bigchannel' successfully created!" "Error while creating channel 'bigchannel'"

joinPeerToChannel 'cli.peer1.org1.samplenet.com' 'bigchannel'
panicOnError $? "Peer joined channel" "Error while joining channel"

installChaincode 'cli.peer1.org1.samplenet.com' 'mycc1' '1.0' 'golang' 'github.com/hyperledger/fabric/chaincodes/go/kv_chaincode_go_example01'
panicOnError $? "Chaincode sucessfully instaled" "Error while installing chaincode"

DATA='{"Args":["init","a","100","b","200"]}'
POLICY='OR("org1MSP.member","org2MSP.member")'
instantiateChaincode 'cli.peer1.org1.samplenet.com' 'orderer1.samplenet.com:7050' true $ORDERER_CA 'bigchannel' 'mycc1' '1.0' $DATA $POLICY
panicOnError $? "Chaincode sucessfully instantiated" "Error while instantiating chaincode"