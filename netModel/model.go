package netModel

import (
	"fmt"
	"github.com/ibm-silvergate/netcomposer/netSpec"
)

type NetModel struct {
	DockerNS             string
	FabricVersionTag     string
	CaVersionTag         string
	ThirdpartyVersionTag string
	ChannelCreationDelay int
	Name                 string
	Domain               string
	Description          string
	OrdererType          string
	KafkaBrokers         []*KafkaBroker
	ZooKeeperNodes       []*ZKNode
	DBProvider           string
	OrdererOrganization  *Organization
	Orderers             []*Orderer
	CAs                  []*CA
	PeerOrganizations    []*Organization
	Channels             map[string]*Channel
	Peers                []*Peer
	Chaincodes           []*Chaincode
	LogLevel             string
	TLSEnabled           bool
}

type Organization struct {
	Name     string
	FullName string
	Peers    []*Peer
}

type Channel struct {
	Name          string
	Organizations []*ChannelOrg
}

type ChannelOrg struct {
	Organization *Organization
	Peers        []*ChannelPeer
}

type ChannelPeer struct {
	Peer           *Peer
	Endorser       bool
	QueryChaincode bool
	QueryLedger    bool
	EventSource    bool
}

type CA struct {
	Name        string
	FullName    string
	OrgFullName string
	ExposedPort int
	Port        int
}

type Orderer struct {
	Name         string
	Organization *Organization
	ExposedPort  int
	Port         int
}

type Peer struct {
	Name                string
	Organization        *Organization
	OrdererOrganization *Organization
	ExposedPort         int
	Port                int
	ExposedEventPort    int
	EventPort           int
	DB                  *PeerDB
}

type PeerDB struct {
	Name        string
	Provider    string
	ExposedPort int
	Port        int
	Namespace   string
	Image       string
	Username    string
	Password    string
	Driver      string
	DB          string
}

type Chaincode struct {
	Name           string
	Channels       []*Channel
	Language       string
	Path           string
	Version        string
	EndorcingRules []*EndorcingRule
}

type EndorcingRule struct {
	Terms []*EndorcingRuleTerm
}

type EndorcingRuleTerm struct {
	Organization *Organization
	Endorsements int
}

type KafkaBroker struct {
	ID   int
	Name string
}

type ZKNode struct {
	ID   int
	Name string
}

func BuildNetModelFrom(spec *netSpec.NetSpec) *NetModel {
	ordererOrganization := &Organization{
		Name:     "ordererOrg",
		FullName: fmt.Sprintf("ordererOrg.%s", spec.Domain),
	}

	ordererList := make([]*Orderer, spec.Orderer.Consenters)
	for i := 0; i < spec.Orderer.Consenters; i++ {
		ordererList[i] = &Orderer{
			Name:         fmt.Sprintf("orderer%d.%s", i+1, spec.Domain),
			Organization: ordererOrganization,
			ExposedPort:  7050 + 100*i,
			Port:         7050,
		}
	}

	peerOrganizationList := make([]*Organization, spec.PeerOrgs)
	caList := make([]*CA, spec.PeerOrgs)
	peerList := make([]*Peer, spec.PeerOrgs*spec.PeersPerOrg)

	for i := 0; i < spec.PeerOrgs; i++ {
		peerOrganizationList[i] = &Organization{
			Name:     fmt.Sprintf("org%d", i+1),
			FullName: fmt.Sprintf("org%d.%s", i+1, spec.Domain),
			Peers:    make([]*Peer, spec.PeersPerOrg),
		}

		caList[i] = &CA{
			Name:        fmt.Sprintf("ca.%s", peerOrganizationList[i].FullName),
			OrgFullName: peerOrganizationList[i].FullName,
			ExposedPort: 7054 + 100*i,
			Port:        7054,
		}

		for j := 0; j < spec.PeersPerOrg; j++ {
			offset := i*spec.PeersPerOrg + j

			dbPort := spec.DB.HostPort + offset
			peerHostPort := 7051 + 10*offset
			eventHostPort := 7053 + 10*offset

			peerdb := &PeerDB{
				Name:        fmt.Sprintf("peer%d.db.%s", j+1, peerOrganizationList[i].FullName),
				Provider:    spec.DB.Provider,
				ExposedPort: dbPort,
				Port:        spec.DB.Port,
				Namespace:   spec.DB.Namespace,
				Image:       spec.DB.Image,
				Username:    spec.DB.Username,
				Password:    spec.DB.Password,
				Driver:      spec.DB.Driver,
				DB:          spec.DB.DB,
			}

			peer := &Peer{
				Name:                fmt.Sprintf("peer%d.%s", j+1, peerOrganizationList[i].FullName),
				Organization:        peerOrganizationList[i],
				OrdererOrganization: ordererOrganization,
				ExposedPort:         peerHostPort,
				Port:                7051,
				ExposedEventPort:    eventHostPort,
				EventPort:           7053,
				DB:                  peerdb,
			}

			peerOrganizationList[i].Peers[j] = peer
			peerList[i*spec.PeersPerOrg+j] = peer
		}
	}

	kafkaBrokerList := make([]*KafkaBroker, spec.Orderer.KafkaBrokers)
	for i := 0; i < spec.Orderer.KafkaBrokers; i++ {
		kafkaBrokerList[i] = &KafkaBroker{
			ID:   i + 1,
			Name: fmt.Sprintf("kafka%d.%s", i+1, spec.Domain),
		}
	}

	zkNodeList := make([]*ZKNode, spec.Orderer.ZookeeperNodes)
	for i := 0; i < spec.Orderer.ZookeeperNodes; i++ {
		zkNodeList[i] = &ZKNode{
			ID:   i + 1,
			Name: fmt.Sprintf("zookeeper%d.%s", i+1, spec.Domain),
		}
	}

	channels := make(map[string]*Channel, len(spec.Channels))
	for _, chSpec := range spec.Channels {
		chOrgList := make([]*ChannelOrg, len(chSpec.Organizations))

		for j, chOrgSpec := range chSpec.Organizations {
			orgPeers := peerOrganizationList[chOrgSpec.ID-1].Peers

			chOrgList[j] = &ChannelOrg{
				Organization: peerOrganizationList[chOrgSpec.ID-1],
				Peers:        make([]*ChannelPeer, len(chOrgSpec.Peers)),
			}

			for p, chPeerSpec := range chOrgSpec.Peers {
				chOrgList[j].Peers[p] = &ChannelPeer{
					Peer:           orgPeers[chPeerSpec.ID-1],
					Endorser:       chPeerSpec.Endorser,
					QueryChaincode: chPeerSpec.QueryChaincode,
					QueryLedger:    chPeerSpec.QueryLedger,
					EventSource:    chPeerSpec.EventSource,
				}
			}
		}

		channels[chSpec.Name] = &Channel{Name: chSpec.Name, Organizations: chOrgList}
	}

	//Build chaincode list solving references (i.e. channels are referenced by name in spec model)
	chaincodeList := make([]*Chaincode, len(spec.Chaincodes))
	for i, ccSpec := range spec.Chaincodes {
		cc := &Chaincode{
			Name:     ccSpec.Name,
			Channels: make([]*Channel, len(ccSpec.Channels)),
			Language: ccSpec.Language,
			Version:  ccSpec.Version,
			Path:     ccSpec.Path,
		}
		//Resolve channel reference by name
		for j, chName := range ccSpec.Channels {
			cc.Channels[j] = channels[chName]
		}
		chaincodeList[i] = cc
	}

	return &NetModel{
		DockerNS:             spec.DockerNS,
		FabricVersionTag:     spec.FabricVersionTag,
		CaVersionTag:         spec.FabricVersionTag,
		ThirdpartyVersionTag: spec.ThirdpartyVersionTag,
		ChannelCreationDelay: spec.ChannelCreationDelay,
		Name:                 spec.Network,
		Domain:               spec.Domain,
		Description:          spec.Description,
		OrdererType:          spec.Orderer.Type,
		KafkaBrokers:         kafkaBrokerList,
		ZooKeeperNodes:       zkNodeList,
		DBProvider:           spec.DB.Provider,
		OrdererOrganization:  ordererOrganization,
		Orderers:             ordererList,
		CAs:                  caList,
		PeerOrganizations:    peerOrganizationList,
		Peers:                peerList,
		Channels:             channels,
		Chaincodes:           chaincodeList,
		LogLevel:             spec.LogLevel,
		TLSEnabled:           spec.TLSEnabled,
	}
}

func (netModel *NetModel) Validate() error {

	for _, ch := range netModel.Channels {
		endorserInChannel := false

		for _, chOrg := range ch.Organizations {
			for _, chPeer := range chOrg.Peers {
				if chPeer.Endorser {
					endorserInChannel = true
					break
				}
			}
			if endorserInChannel {
				break
			}
		}

		if !endorserInChannel {
			return fmt.Errorf("Channel '%s' does not specify any endorsing peer", ch.Name)
		}
	}

	return nil
}
