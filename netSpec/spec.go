package netSpec

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"

	yaml "gopkg.in/yaml.v2"
)

//Constants used to identify DBProvider and Ordering Service
const (
	DBProviderGoLevelDB string = "goleveldb"
	DBProviderCouchDB   string = "CouchDB"

	OrderingServiceSOLO  string = "solo"
	OrderingServiceKafKa string = "kafka"
)

type NetSpec struct {
	DockerNS             string           `yaml:"DOCKER_NS"`
	FabricVersionTag     string           `yaml:"FABRIC_VERSION_TAG"`
	CaVersionTag         string           `yaml:"CA_VERSION_TAG"`
	ThirdpartyVersionTag string           `yaml:"THIRDPARTY_VERSION_TAG"`
	ChannelCreationDelay int			  `yaml:"CHANNEL_CREATION_DELAY"`
	Network              string           `yaml:"network"`
	Domain               string           `yaml:"domain"`
	Description          string           `yaml:"description"`
	Orderer              *OrdererSpec     `yaml:"orderer"`
	DB                   *DBSpec          `yaml:"db"`
	PeerOrgs             int              `yaml:"organizations"`
	PeersPerOrg          int              `yaml:"peersPerOrganization"`
	PeerOrgUsers         int              `yaml:"usersPerOrganization"`
	Channels             []*ChannelSpec   `yaml:"channels"`
	LogLevel             string           `yaml:"logLevel"`
	TLSEnabled           bool             `yaml:"tlsEnabled"`
	ChaincodesPath       string           `yaml:"chaincodesPath"`
	Chaincodes           []*ChaincodeSpec `yaml:"chaincodes"`
}

type OrdererSpec struct {
	Type           string `yaml:"type"`
	Consenters     int    `yaml:"consenters"`
	KafkaBrokers   int    `yaml:"kafkaBrokers"`
	ZookeeperNodes int    `yaml:"zookeeperNodes"`
}

type ChannelSpec struct {
	Name          string            `yaml:"name"`
	Organizations []*ChannelOrgSpec `yaml:"organizations"`
}

type ChannelOrgSpec struct {
	ID    int                `yaml:"organization"`
	Peers []*ChannelPeerSpec `yaml:"peers"`
}

type ChannelPeerSpec struct {
	ID             int  `yaml:"peer"`
	Endorser       bool `yaml:"endorser"`
	QueryChaincode bool `yaml:"queryChaincode"`
	QueryLedger    bool `yaml:"queryLedger"`
	EventSource    bool `yaml:"eventSource"`
}

type DBSpec struct {
	Provider  string `yaml:"provider"`
	Port      int    `yaml:"port"`
	HostPort  int    `yaml:"hostPort"`
	Namespace string `yaml:"namespace"`
	Image     string `yaml:"image"`
	Username  string `yaml:"username"`
	Password  string `yaml:"password"`
	Driver    string `yaml:"driver"`
	DB        string `yaml:"db"`
}

type ChaincodeSpec struct {
	Name           string `yaml:"name"`
	Channels       []string
	Language       string `yaml:"language"`
	Path           string `yaml:"path"`
	Version        string `yaml:"version"`
	EndorcingRules []*EndorcingRuleSpec
}

type EndorcingRuleSpec struct {
	Terms []*EndorcingRuleTermSpec
}

type EndorcingRuleTermSpec struct {
	Organization string `yaml:"organization"`
	Endorsements int    `yaml:"endorsements"`
}

func LoadFromFile(specFile string) (*NetSpec, error) {
	yamlFile, err := ioutil.ReadFile(specFile)
	if err != nil {
		log.Printf("Error reading net specification file:   #%v ", err)
		return nil, err
	}

	spec := &NetSpec{}
	err = yaml.Unmarshal(yamlFile, spec)
	if err != nil {
		log.Fatalf("Unmarshal: %v", err)
		return nil, err
	}
	return spec, nil
}

func (spec *NetSpec) SetDefaults() {
	/* This step is required when using SOLO ordering service
	 * Consenters field is optional is such case
	 */
	if spec.Orderer.Consenters < 1 {
		spec.Orderer.Consenters = 1
	}

	// Set default ports for CouchDB when not specified in config file
	if spec.DB.Provider == DBProviderCouchDB {
		if spec.DB.Port == 0 {
			spec.DB.Port = 5984
		}
		if spec.DB.HostPort == 0 {
			spec.DB.HostPort = 5984
		}
	}

	for _, chSpec := range spec.Channels {
		//DEFAULT: when no organizations are specified for the channel, it means all organizations
		if chSpec.Organizations == nil || len(chSpec.Organizations) == 0 {
			chSpec.Organizations = make([]*ChannelOrgSpec, spec.PeerOrgs)
			for i := 0; i < spec.PeerOrgs; i++ {
				chSpec.Organizations[i] = &ChannelOrgSpec{ID: i + 1}
			}
		}

		for _, chOrgSpec := range chSpec.Organizations {
			//DEFAULT: specify all peers as endorsers if no peer was specified
			if chOrgSpec.Peers == nil || len(chOrgSpec.Peers) == 0 {
				chOrgSpec.Peers = make([]*ChannelPeerSpec, spec.PeersPerOrg)
				for p := 0; p < spec.PeersPerOrg; p++ {
					chOrgSpec.Peers[p] = &ChannelPeerSpec{
						ID:             p + 1,
						Endorser:       true,
						QueryChaincode: true,
						QueryLedger:    true,
						EventSource:    true,
					}
				}
			}
		}

	}
}

func (spec *NetSpec) Validate() error {
	if spec.DockerNS == "" {
		return errors.New("DOCKER_NS must be specified")
	}

	if spec.FabricVersionTag == "" {
		return errors.New("FABRIC_VERSION_TAG must be specified")
	}

	if spec.CaVersionTag == "" {
		return errors.New("CA_VERSION_TAG must be specified")
	}

	if spec.ThirdpartyVersionTag == "" {
		return errors.New("THIRDPARTY_VERSION_TAG must be specified")
	}

	if spec.Orderer.Type != OrderingServiceSOLO && spec.Orderer.Type != OrderingServiceKafKa {
		return fmt.Errorf("Unsupported orderer type '%s'", spec.Orderer.Type)
	}

	if spec.Orderer.Type == OrderingServiceKafKa && spec.Orderer.Consenters <= 0 {
		return fmt.Errorf("A positive number of orderer nodes (consenters) is required if orderer type is '%s'", spec.Orderer.Type)
	}

	if spec.Orderer.Type == OrderingServiceKafKa && spec.Orderer.KafkaBrokers < 1 {
		return fmt.Errorf("A positive number of brokers is required if orderer type is %s", spec.Orderer.Type)
	}

	if spec.Orderer.Type == OrderingServiceKafKa && spec.Orderer.ZookeeperNodes < 1 {
		return fmt.Errorf("A positive number of zookeeper nodes is required if orderer type is '%s'", spec.Orderer.Type)
	}

	if spec.DB.Provider != DBProviderGoLevelDB && spec.DB.Provider != DBProviderCouchDB {
		log.Printf("Warnning: using unofficial db provider  '%s'\r\n", spec.DB.Provider)
	}

	if spec.PeerOrgs <= 0 {
		return errors.New("Number of peer organization must be greater than 0")
	}

	if spec.PeerOrgUsers < 0 {
		return errors.New("Number of user peers per organization must be non negative")
	}

	for _, chSpec := range spec.Channels {
		if chSpec.Organizations == nil || len(chSpec.Organizations) == 0 {
			return fmt.Errorf("Channel '%s' has not specified any organization", chSpec.Name)
		}

		for _, chOrgSpec := range chSpec.Organizations {
			if chOrgSpec.ID < 1 || chOrgSpec.ID > spec.PeerOrgs {
				return fmt.Errorf("Invalid organization ID '%d' specified for channel '%s'", chOrgSpec.ID, chSpec.Name)
			}

			if chOrgSpec.Peers == nil || len(chOrgSpec.Peers) == 0 {
				return fmt.Errorf("Channel '%s' has not specified any peer for organization '%d'", chSpec.Name, chOrgSpec.ID)
			}

			for _, chPeerSpec := range chOrgSpec.Peers {
				if chPeerSpec.ID < 1 || chPeerSpec.ID > spec.PeersPerOrg {
					return fmt.Errorf("Invalid peer ID '%d' specified for organization '%d' in channel '%s'", chPeerSpec.ID, chOrgSpec.ID, chSpec.Name)
				}
			}
		}
	}

	//TODO: validate chaincodes, including references to nonexistent channels

	return nil
}
