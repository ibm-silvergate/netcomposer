/*
Copyright IBM Corp. 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"gopkg.in/yaml.v2"
)

//Constants used to identify DBProvider and Ordering Service
const (
	DBProviderGoLevelDB string = "goleveldb"
	DBProviderCouchDB   string = "CouchDB"

	OrderingServiceSOLO  string = "solo"
	OrderingServiceKafKa string = "kafka"
)

type configuration struct {
	DockerNS       string        `yaml:"DOCKER_NS"`
	Arch           string        `yaml:"ARCH"`
	Version        string        `yaml:"VERSION"`
	Network        string        `yaml:"network"`
	Domain         string        `yaml:"domain"`
	Description    string        `yaml:"description"`
	Orderer        ordererSpec   `yaml:"orderer"`
	DB             dbSpec        `yaml:"db"`
	PeerOrgs       int           `yaml:"peerOrganizations"`
	PeersPerOrg    int           `yaml:"peersPerOrganization"`
	PeerOrgUsers   int           `yaml:"usersPerOrganization"`
	Channels       []channelSpec `yaml:"channels"`
	LogLevel       string        `yaml:"logLevel"`
	TLSEnabled     bool          `yaml:"tlsEnabled"`
	ChaincodesPath string        `yaml:"chaincodesPath"`
}

type ordererSpec struct {
	Type           string `yaml:"type"`
	Consenters     int    `yaml:"consenters"`
	KafkaBrokers   int    `yaml:"kafkaBrokers"`
	ZookeeperNodes int    `yaml:"zookeeperNodes"`
}

type channelSpec struct {
	Name          string           `yaml:"name"`
	Organizations []channelOrgSpec `yaml:"organizations"`
}

type channelOrgSpec struct {
	ID    int               `yaml:"organization"`
	Peers []channelPeerSpec `yaml:"peers"`
}

type channelPeerSpec struct {
	ID             int  `yaml:"peer"`
	Endorser       bool `yaml:"endorser"`
	QueryChaincode bool `yaml:"queryChaincode"`
	QueryLedger    bool `yaml:"queryLedger"`
	EventSource    bool `yaml:"eventSource"`
}

type dbSpec struct {
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

type genInfo struct {
	DockerNS            string
	Arch                string
	Version             string
	Name                string
	Domain              string
	Description         string
	OrdererType         string
	KafkaBrokers        []kafkaBroker
	ZooKeeperNodes      []zkNode
	DBProvider          string
	OrdererOrganization organization
	Orderers            []orderer
	CAs                 []ca
	PeerOrganizations   []organization
	Channels            []channel
	Peers               []peer
	LogLevel            string
	TLSEnabled          bool
}

type organization struct {
	Name     string
	FullName string
	Peers    []peer
}

type channel struct {
	Name          string
	Organizations []channelOrg
}

type channelOrg struct {
	Name  string
	Peers []channelPeer
}

type channelPeer struct {
	Name           string
	Endorser       bool
	QueryChaincode bool
	QueryLedger    bool
	EventSource    bool
}

type ca struct {
	Name        string
	FullName    string
	OrgFullName string
	ExposedPort int
	Port        int
}

type orderer struct {
	Name         string
	Organization organization
	ExposedPort  int
	Port         int
}

type peer struct {
	Name                string
	Organization        organization
	OrdererOrganization organization
	ExposedPort         int
	Port                int
	ExposedEventPort    int
	EventPort           int
	DB                  peerdb
}

type peerdb struct {
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

type kafkaBroker struct {
	ID   int
	Name string
}

type zkNode struct {
	ID   int
	Name string
}

var (
	configFile       string
	config           *configuration
	volumesPath      string
	cryptoConfigPath string
	genesisPath      string
	channelsPath     string
	networkPath      string
)

func (c *configuration) readConfig(configFile string) *configuration {
	yamlFile, err := ioutil.ReadFile(configFile)
	if err != nil {
		log.Printf("Error reading config file:   #%v ", err)
	}
	err = yaml.Unmarshal(yamlFile, c)
	if err != nil {
		log.Fatalf("Unmarshal: %v", err)
	}
	return c
}

func loadConfig() *configuration {
	flag.StringVar(&configFile, "config", "", "config file e.g. samplenet.yaml")
	flag.Parse()

	if configFile == "" {
		fmt.Fprintln(os.Stderr, "config file must be specified")
		os.Exit(1)
	}

	config = &configuration{}
	config.readConfig(configFile)

	if config.DockerNS == "" {
		fmt.Fprintln(os.Stderr, "DOCKER_NS must be specified")
		os.Exit(1)
	}

	if config.Arch == "" {
		fmt.Fprintln(os.Stderr, "ARCH must be specified")
		os.Exit(1)
	}

	if config.Version == "" {
		fmt.Fprintln(os.Stderr, "VERSION must be specified")
		os.Exit(1)
	}

	if config.Orderer.Type != OrderingServiceSOLO && config.Orderer.Type != OrderingServiceKafKa {
		fmt.Fprintln(os.Stderr, fmt.Sprintf("Unsupported orderer type %s", config.Orderer.Type))
		os.Exit(1)
	}

	if config.Orderer.Type == OrderingServiceKafKa && config.Orderer.Consenters <= 0 {
		fmt.Fprintln(os.Stderr, fmt.Sprintf("A positive number of orderer nodes (consenters) is required if orderer type is %s", config.Orderer.Type))
		os.Exit(1)
	}

	if config.Orderer.Type == OrderingServiceKafKa && config.Orderer.KafkaBrokers < 1 {
		fmt.Fprintln(os.Stderr, fmt.Sprintf("A positive number of brokers is required if orderer type is %s", config.Orderer.Type))
		os.Exit(1)
	}

	if config.Orderer.Type == OrderingServiceKafKa && config.Orderer.ZookeeperNodes < 1 {
		fmt.Fprintln(os.Stderr, fmt.Sprintf("A positive number of zookeeper nodes is required if orderer type is %s", config.Orderer.Type))
		os.Exit(1)
	}

	if config.DB.Provider != DBProviderGoLevelDB && config.DB.Provider != DBProviderCouchDB {
		fmt.Fprintln(os.Stderr, fmt.Sprintf("Unsupported db provider  %s", config.DB.Provider))
		os.Exit(1)
	}

	if config.PeerOrgs <= 0 {
		fmt.Fprintln(os.Stderr, "Number of peer organziation must be greater than 0")
		os.Exit(1)
	}

	if config.PeerOrgUsers <= 0 {
		fmt.Fprintln(os.Stderr, "Number of peer per organziation must be greater than 0")
		os.Exit(1)
	}

	if config.PeerOrgUsers < 0 {
		fmt.Fprintln(os.Stderr, "Number of user peers per organziation must be non negative")
		os.Exit(1)
	}

	return config
}

func main() {

	loadConfig()

	volumesPath = filepath.Join(config.Network, "volumes")
	cryptoConfigPath = filepath.Join(volumesPath, "crypto-config")
	genesisPath = filepath.Join(cryptoConfigPath, "genesis")
	channelsPath = filepath.Join(cryptoConfigPath, "channel-artifacts")
	networkPath = filepath.Join(volumesPath, "network")

	os.MkdirAll(config.Network, 0777)
	os.MkdirAll(genesisPath, 0777)
	os.MkdirAll(channelsPath, 0777)
	os.MkdirAll(networkPath, 0777)

	/* This step is required when using SOLO ordering service
	 * Consenters field is optional is such case
	 */
	if config.Orderer.Consenters < 1 {
		config.Orderer.Consenters = 1
	}

	// Set default ports for CouchDB when not specified in config file
	if config.DB.Provider == DBProviderCouchDB {
		if config.DB.Port == 0 {
			config.DB.Port = 5984
		}
		if config.DB.HostPort == 0 {
			config.DB.HostPort = 5984
		}
	}

	ordererOrganization := &organization{
		Name:     "ordererOrg",
		FullName: fmt.Sprintf("ordererOrg.%s", config.Domain),
	}

	ordererList := make([]orderer, config.Orderer.Consenters)
	for i := 0; i < config.Orderer.Consenters; i++ {
		ordererList[i] = orderer{
			Name:         fmt.Sprintf("orderer%d.%s", i+1, config.Domain),
			Organization: *ordererOrganization,
			ExposedPort:  7050 + 100*i,
			Port:         7050,
		}
	}

	peerOrganizationList := make([]organization, config.PeerOrgs)
	caList := make([]ca, config.PeerOrgs)
	peerList := make([]peer, config.PeerOrgs*config.PeersPerOrg)

	for i := 0; i < config.PeerOrgs; i++ {
		peerOrganizationList[i] = organization{
			Name:     fmt.Sprintf("org%d", i+1),
			FullName: fmt.Sprintf("org%d.%s", i+1, config.Domain),
			Peers:    make([]peer, config.PeersPerOrg),
		}

		caList[i] = ca{
			Name:        fmt.Sprintf("ca.%s", peerOrganizationList[i].FullName),
			OrgFullName: peerOrganizationList[i].FullName,
			ExposedPort: 7054 + 100*i,
			Port:        7054,
		}

		for j := 0; j < config.PeersPerOrg; j++ {
			offset := i*config.PeersPerOrg + j

			dbPort := config.DB.HostPort + offset
			peerHostPort := 7051 + 10*offset
			eventHostPort := 7053 + 10*offset

			peerdb := &peerdb{
				Name:        fmt.Sprintf("peer%d.db.%s", j+1, peerOrganizationList[i].FullName),
				Provider:    config.DB.Provider,
				ExposedPort: dbPort,
				Port:        config.DB.Port,
				Namespace:   config.DB.Namespace,
				Image:       config.DB.Image,
				Username:    config.DB.Username,
				Password:    config.DB.Password,
				Driver:      config.DB.Driver,
				DB:          config.DB.DB,
			}

			peer := peer{
				Name:                fmt.Sprintf("peer%d.%s", j+1, peerOrganizationList[i].FullName),
				Organization:        peerOrganizationList[i],
				OrdererOrganization: *ordererOrganization,
				ExposedPort:         peerHostPort,
				Port:                7051,
				ExposedEventPort:    eventHostPort,
				EventPort:           7053,
				DB:                  *peerdb,
			}

			peerOrganizationList[i].Peers[j] = peer
			peerList[i*config.PeersPerOrg+j] = peer
		}
	}

	kafkaBrokerList := make([]kafkaBroker, config.Orderer.KafkaBrokers)
	for i := 0; i < config.Orderer.KafkaBrokers; i++ {
		kafkaBrokerList[i] = kafkaBroker{
			ID:   i + 1,
			Name: fmt.Sprintf("kafka%d.%s", i+1, config.Domain),
		}
	}

	zkNodeList := make([]zkNode, config.Orderer.ZookeeperNodes)
	for i := 0; i < config.Orderer.ZookeeperNodes; i++ {
		zkNodeList[i] = zkNode{
			ID:   i + 1,
			Name: fmt.Sprintf("zookeeper%d.%s", i+1, config.Domain),
		}
	}

	channelList := make([]channel, len(config.Channels))
	for i, chSpec := range config.Channels {
		//DEFAULT: when no organizations are specified for the channel, it means all organizations
		if chSpec.Organizations == nil || len(chSpec.Organizations) == 0 {
			chSpec.Organizations = make([]channelOrgSpec, len(peerOrganizationList))
			for i := 0; i < len(peerOrganizationList); i++ {
				chSpec.Organizations[i] = channelOrgSpec{ID: i + 1}
			}
		}

		chOrgList := make([]channelOrg, len(chSpec.Organizations))
		for j, chOrgSpec := range chSpec.Organizations {
			if chOrgSpec.ID < 1 || chOrgSpec.ID > len(peerOrganizationList) {
				panic(fmt.Sprintf("Invalid organization ID (%d) specified for channel %s", chOrgSpec.ID, chSpec.Name))
			}

			chOrgList[j] = channelOrg{Name: peerOrganizationList[chOrgSpec.ID-1].Name}

			orgPeers := peerOrganizationList[chOrgSpec.ID-1].Peers

			//DEFAULT: specify all peers as endorsers if no peer was specified
			if chOrgSpec.Peers == nil || len(chOrgSpec.Peers) == 0 {
				chOrgSpec.Peers = make([]channelPeerSpec, len(orgPeers))
				for p := 0; p < len(orgPeers); p++ {
					chOrgSpec.Peers[p] = channelPeerSpec{
						ID:             p + 1,
						Endorser:       true,
						QueryChaincode: true,
						QueryLedger:    true,
						EventSource:    true,
					}
				}
			}

			//Assign peer names
			if chOrgSpec.Peers != nil {
				chOrgList[j].Peers = make([]channelPeer, len(chOrgSpec.Peers))

				for p, chPeerSpec := range chOrgSpec.Peers {
					if chPeerSpec.ID < 1 || chPeerSpec.ID > len(orgPeers) {
						panic(fmt.Sprintf("Invalid peer ID (%d) specified for channel %s", chPeerSpec.ID, chSpec.Name))
					}
					chOrgList[j].Peers[p] = channelPeer{
						Name:           orgPeers[chPeerSpec.ID-1].Name,
						Endorser:       chPeerSpec.Endorser,
						QueryChaincode: chPeerSpec.QueryChaincode,
						QueryLedger:    chPeerSpec.QueryLedger,
						EventSource:    chPeerSpec.EventSource,
					}

				}
			}
		}

		channelList[i] = channel{Name: chSpec.Name, Organizations: chOrgList}
	}

	genInfo := &genInfo{
		DockerNS:            config.DockerNS,
		Arch:                config.Arch,
		Version:             config.Version,
		Name:                config.Network,
		Domain:              config.Domain,
		Description:         config.Description,
		OrdererType:         config.Orderer.Type,
		KafkaBrokers:        kafkaBrokerList,
		ZooKeeperNodes:      zkNodeList,
		DBProvider:          config.DB.Provider,
		OrdererOrganization: *ordererOrganization,
		Orderers:            ordererList,
		CAs:                 caList,
		PeerOrganizations:   peerOrganizationList,
		Peers:               peerList,
		Channels:            channelList,
		LogLevel:            config.LogLevel,
		TLSEnabled:          config.TLSEnabled,
	}

	copyChaincodes(config)

	generateCryptoConfigFile(config)

	generateCryptoMaterial(genInfo, "crypto-config.yaml")
	/* Fix naming from cryptogen tool
	* Rename private key files ending in "_sk" to "secret.key" for easier configuration in templates
	 */
	filepath.Walk(filepath.Join(volumesPath, "crypto-config"), fixSKFilename)

	generateConfigTXFile(genInfo)

	generateDockerComposeFile(genInfo)

	generatePullImagesScriptFile(genInfo)

	generateNetworkConfigFile(genInfo)

	generateNetworkConfigForOrgs(genInfo)

	generateGenesisBlock(genesisPath, "genesis.block")

	generateChannelConfig(genInfo, channelsPath)
}

func generateCryptoConfigFile(config *configuration) {
	fmt.Print("Generating crypto config file: ")
	cryptoConfigTemplate := loadTemplate("crypto-config-template.yaml")
	execTemplate(cryptoConfigTemplate, config, config.Network, "crypto-config.yaml")
	fmt.Println("SUCCEED")
}

func generateConfigTXFile(genInfo *genInfo) {
	fmt.Print("Generating configTX file: ")
	configTXTemplate := loadTemplate("configtx-template.yaml")
	execTemplate(configTXTemplate, genInfo, genInfo.Name, "configtx.yaml")
	fmt.Println("SUCCEED")
}

func generateDockerComposeFile(genInfo *genInfo) {
	fmt.Print("Generating docker compose file: ")
	dockerComposeTemplate := loadTemplate("docker-compose-template.yaml")
	execTemplate(dockerComposeTemplate, genInfo, genInfo.Name, "docker-compose.yaml")
	fmt.Println("SUCCEED")
}

func generateNetworkConfigFile(genInfo *genInfo) {
	fmt.Print("Generating network config file: ")
	networkConfigTemplate := loadTemplate("network-config-template.yaml")
	execTemplate(networkConfigTemplate, genInfo, networkPath, "network-config.yaml")
	fmt.Println("SUCCEED")
}

func generateNetworkConfigForOrgs(genInfo *genInfo) {
	networkConfigTemplate := loadTemplate("network-config-org-template.yaml")

	for _, org := range genInfo.PeerOrganizations {
		fmt.Printf("Generating network config for organization %s: ", org.Name)
		genInfoClientDef := struct {
			Network      string
			Description  string
			Organization string
		}{
			Network:      genInfo.Name,
			Description:  genInfo.Description,
			Organization: org.Name,
		}

		execTemplate(networkConfigTemplate, genInfoClientDef, networkPath, fmt.Sprintf("network-config-%s.yaml", org.Name))
		fmt.Println("SUCCEED")
	}
}

func generatePullImagesScriptFile(genInfo *genInfo) {
	fmt.Print("Generating script to pull fabric docker images: ")
	pullImagesTemplate := loadTemplate("pull-docker-images-template.yaml")
	execTemplate(pullImagesTemplate, genInfo, genInfo.Name, "pull-docker-images.sh")
	args := []string{"+x", filepath.Join(genInfo.Name, "pull-docker-images.sh")}
	if err := exec.Command("chmod", args...).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("SUCCEED")
}

func fixSKFilename(path string, f os.FileInfo, err error) (e error) {
	if strings.HasSuffix(f.Name(), "_sk") {
		dir := filepath.Dir(path)
		newname := filepath.Join(dir, "secret.key")
		os.Rename(path, newname)
	}
	return
}

func architecture() string {
	arch, err := exec.Command("uname", "-s").Output()

	if err != nil {
		log.Fatal(err)
	}

	sarch := strings.ToLower(strings.TrimSpace(string(arch)))

	return strings.ToLower(fmt.Sprintf("%s", sarch)) + "-amd64"
}

func generateCryptoMaterial(genInfo *genInfo, cryptoConfigFile string) {
	fmt.Print("Generating crypto material: ")
	cryptoConfigFilePath := filepath.Join(genInfo.Name, cryptoConfigFile)

	args := []string{
		"generate",
		"--config", cryptoConfigFilePath,
		"--output", cryptoConfigPath,
	}

	if err := exec.Command(fmt.Sprintf("./tools/%s/cryptogen", architecture()), args...).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("SUCCEED")
}

func generateGenesisBlock(genesisPath, genesisFile string) {
	fmt.Print("Generating genesis block: ")

	args := []string{
		"-profile", config.Network + "Genesis",
		"-outputBlock", filepath.Join(genesisPath, genesisFile),
	}

	cmd := exec.Command(fmt.Sprintf("./tools/%s/configtxgen", architecture()), args...)
	cmd.Env = os.Environ()
	pwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cmd.Env = append(cmd.Env, fmt.Sprintf("FABRIC_CFG_PATH=%s", filepath.Join(pwd, config.Network)))

	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("SUCCEED")
}

func generateChannelConfig(genInfo *genInfo, channelsPath string) {
	for _, ch := range genInfo.Channels {
		fmt.Printf("Generating config for channel %s: ", ch.Name)

		args := []string{
			"-profile", ch.Name,
			"-outputCreateChannelTx", filepath.Join(channelsPath, fmt.Sprintf("%s.tx", ch.Name)),
			"-channelID", ch.Name,
		}

		cmd := exec.Command(fmt.Sprintf("./tools/%s/configtxgen", architecture()), args...)
		cmd.Env = os.Environ()
		pwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		cmd.Env = append(cmd.Env, fmt.Sprintf("FABRIC_CFG_PATH=%s", filepath.Join(pwd, config.Network)))

		if err := cmd.Run(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("SUCCEED")
	}
}

func copyChaincodes(config *configuration) {
	if config.ChaincodesPath != "" {
		fmt.Print("Copying chaincodes to volumes: ")
		copyFolder(config.ChaincodesPath, filepath.Join(config.Network, "volumes/chaincodes"))
		fmt.Println("SUCCEED")
	} else {
		fmt.Println("Chaincodes path was not specified, no chaincode will be included into peer containers")
	}
}

func copyFolder(sPath, dPath string) {
	cpArgs := []string{"-r", sPath, dPath}
	if err := exec.Command("cp", cpArgs...).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func loadTemplate(templateFile string) *template.Template {
	templateFilePath := path.Join("templates", templateFile)

	fm := template.FuncMap{
		"Sequence": sequence,
		"ToLower":  strings.ToLower,
		"Inc":      inc,
	}

	t, err := template.New(templateFile).Funcs(fm).ParseFiles(templateFilePath)
	if err != nil {
		log.Fatalln(err)
	}
	return t
}

func sequence(start, end int) (stream chan int) {
	stream = make(chan int)
	go func() {
		for i := start; i <= end; i++ {
			stream <- i
		}
		close(stream)
	}()
	return
}

func inc(val int) int {
	return val + 1
}

func execTemplate(t *template.Template, genInfo interface{}, targetPath string, targetFile string) error {
	path := filepath.Join(targetPath, targetFile)

	f, e := os.Create(path)
	if e != nil {
		log.Println("Error creating file: ", e)
		return e
	}

	e = t.Execute(f, genInfo)
	if e != nil {
		log.Println("Error executing template: ", e)
		return e
	}

	return nil
}
