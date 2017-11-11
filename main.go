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

type configuration struct {
	DockerNS       string      `yaml:"DOCKER_NS"`
	Arch           string      `yaml:"ARCH"`
	Version        string      `yaml:"VERSION"`
	Network        string      `yaml:"network"`
	Domain         string      `yaml:"domain"`
	Orderer        ordererSpec `yaml:"orderer"`
	DB             dbSpec      `yaml:"db"`
	PeerOrgs       int         `yaml:"peerOrganizations"`
	PeersPerOrg    int         `yaml:"peersPerOrganization"`
	PeerOrgUsers   int         `yaml:"usersPerOrganization"`
	LogLevel       string      `yaml:"logLevel"`
	TLSEnabled     bool        `yaml:"tlsEnabled"`
	ChaincodesPath string      `yaml:"chaincodesPath"`
}

type ordererSpec struct {
	Type           string `yaml:"type"`
	Consenters     int    `yaml:"consenters"`
	KafkaBrokers   int    `yaml:"kafkaBrokers"`
	ZookeeperNodes int    `yaml:"zookeeperNodes"`
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
	OrdererType         string
	KafkaBrokers        []kafkaBroker
	ZooKeeperNodes      []zkNode
	DBProvider          string
	OrdererOrganization organization
	Orderers            []orderer
	PeerOrganizations   []organization
	Peers               []peer
	LogLevel            string
	TLSEnabled          bool
}

type organization struct {
	Name   string
	Domain string
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

	if config.Orderer.Type != "solo" && config.Orderer.Type != "kafka" {
		fmt.Fprintln(os.Stderr, fmt.Sprintf("Unsupported orderer type %s", config.Orderer.Type))
		os.Exit(1)
	}

	if config.Orderer.Type == "kafka" && config.Orderer.Consenters <= 0 {
		fmt.Fprintln(os.Stderr, fmt.Sprintf("A positive number of orderer nodes (consenters) is required if orderer type is %s", config.Orderer.Type))
		os.Exit(1)
	}

	if config.Orderer.Type == "kafka" && config.Orderer.KafkaBrokers < 1 {
		fmt.Fprintln(os.Stderr, fmt.Sprintf("A positive number of brokers is required if orderer type is %s", config.Orderer.Type))
		os.Exit(1)
	}

	if config.Orderer.Type == "kafka" && config.Orderer.ZookeeperNodes < 1 {
		fmt.Fprintln(os.Stderr, fmt.Sprintf("A positive number of zookeeper nodes is required if orderer type is %s", config.Orderer.Type))
		os.Exit(1)
	}

	if config.DB.Provider != "goleveldb" && config.DB.Provider != "CouchDB" {
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

	os.MkdirAll(genesisPath, 0777)
	os.MkdirAll(channelsPath, 0777)

	if config.Orderer.Consenters < 1 {
		config.Orderer.Consenters = 1
	}

	cryptoConfigTemplate := loadTemplate(config, "crypto-config-template.yaml")
	execTemplateWithConfig(cryptoConfigTemplate, config, "crypto-config.yaml")

	generateCryptoMaterial(config, "crypto-config.yaml")

	copyChaincodes(config)

	configTXTemplate := loadTemplate(config, "configtx-template.yaml")
	dockerComposeTemplate := loadTemplate(config, "docker-compose-template.yaml")

	ordererOrganization := &organization{
		Name:   "ordererOrg",
		Domain: config.Domain,
	}

	ordererList := make([]orderer, config.Orderer.Consenters)
	for i := 0; i < config.Orderer.Consenters; i++ {
		ordererList[i] = orderer{
			Name:         fmt.Sprintf("orderer%d.%s", i+1, ordererOrganization.Domain),
			Organization: *ordererOrganization,
			ExposedPort:  7050 + 100*i,
			Port:         7050,
		}
	}

	peerOrganizationList := make([]organization, config.PeerOrgs)
	peerList := make([]peer, config.PeerOrgs*config.PeersPerOrg)

	for i := 0; i < config.PeerOrgs; i++ {
		peerOrganizationList[i] = organization{
			Name:   fmt.Sprintf("org%d", i+1),
			Domain: fmt.Sprintf("org%d.%s", i+1, config.Domain),
		}

		for j := 0; j < config.PeersPerOrg; j++ {
			offset := i*config.PeersPerOrg + j

			dbPort := config.DB.HostPort + offset
			peerHostPort := 7051 + 10*offset
			eventHostPort := 7053 + 10*offset

			peerdb := &peerdb{
				Name:        fmt.Sprintf("peer%d.db.%s", j+1, peerOrganizationList[i].Domain),
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

			peerList[i*config.PeersPerOrg+j] = peer{
				Name:                fmt.Sprintf("peer%d.%s", j+1, peerOrganizationList[i].Domain),
				Organization:        peerOrganizationList[i],
				OrdererOrganization: *ordererOrganization,
				ExposedPort:         peerHostPort,
				Port:                7051,
				ExposedEventPort:    eventHostPort,
				EventPort:           7053,
				DB:                  *peerdb,
			}
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

	genInfo := &genInfo{
		DockerNS:            config.DockerNS,
		Arch:                config.Arch,
		Version:             config.Version,
		Name:                config.Network,
		OrdererType:         config.Orderer.Type,
		KafkaBrokers:        kafkaBrokerList,
		ZooKeeperNodes:      zkNodeList,
		DBProvider:          config.DB.Provider,
		OrdererOrganization: *ordererOrganization,
		Orderers:            ordererList,
		PeerOrganizations:   peerOrganizationList,
		Peers:               peerList,
		LogLevel:            config.LogLevel,
		TLSEnabled:          config.TLSEnabled,
	}

	execTemplate(configTXTemplate, genInfo, config, "configtx.yaml")
	execTemplate(dockerComposeTemplate, genInfo, config, "docker-compose.yaml")

	generateGenesisBlock(config, genesisPath, "genesis.block")

	generateChannelConfig(config, channelsPath, "bigchannel.tx")

	fmt.Print("> Generating script to pull fabric docker images... ")
	pullImagesTemplate := loadTemplate(config, "pull-docker-images-template.yaml")
	execTemplateWithConfig(pullImagesTemplate, config, "pull-docker-images.sh")
	args := []string{"+x", filepath.Join(config.Network, "pull-docker-images.sh")}
	if err := exec.Command("chmod", args...).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Success!")
}

func architecture() string {
	arch, err := exec.Command("uname", "-s").Output()

	if err != nil {
		log.Fatal(err)
	}

	sarch := strings.ToLower(strings.TrimSpace(string(arch)))

	return strings.ToLower(fmt.Sprintf("%s", sarch)) + "-amd64"
}

func generateCryptoMaterial(config *configuration, cryptoConfigFile string) {
	fmt.Print("> Generating crypto material...")
	cryptoConfigFilePath := filepath.Join(config.Network, cryptoConfigFile)

	args := []string{
		"generate",
		"--config", cryptoConfigFilePath,
		"--output", cryptoConfigPath,
	}

	if err := exec.Command(fmt.Sprintf("./tools/%s/cryptogen", architecture()), args...).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Success!")
}

func generateGenesisBlock(config *configuration, genesisPath, genesisFile string) {
	fmt.Print("> Generating genesis block...")

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
	fmt.Println("Success!")
}

func generateChannelConfig(config *configuration, channelsPath, channelFile string) {
	fmt.Print("> Generating global channel config...")

	args := []string{
		"-profile", config.Network + "Channel",
		"-outputCreateChannelTx", filepath.Join(channelsPath, channelFile),
		"-channelID", "bigchannel",
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
	fmt.Println("Success!")
}

func copyChaincodes(config *configuration) {
	if config.ChaincodesPath != "" {
		fmt.Print("> Copying chaincodes to volumes...")
		copyFolder(config.ChaincodesPath, filepath.Join(config.Network, "volumes/chaincodes"))
		fmt.Println("Success!")
	} else {
		fmt.Println("> Chaincodes path was not specified, no chaincode will be included into peer containers")
	}
}

func copyFolder(sPath, dPath string) {
	cpArgs := []string{"-r", sPath, dPath}
	if err := exec.Command("cp", cpArgs...).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func loadTemplate(config *configuration, templateFile string) *template.Template {
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

func execTemplate(t *template.Template, gi *genInfo, c *configuration, targetFile string) error {
	os.MkdirAll(c.Network, 0777)

	path := filepath.Join(c.Network, targetFile)

	f, e := os.Create(path)
	if e != nil {
		log.Println("Error creating file: ", e)
		return e
	}

	e = t.Execute(f, gi)
	if e != nil {
		log.Println("Error executing template: ", e)
		return e
	}

	return nil
}

func execTemplateWithConfig(t *template.Template, c *configuration, targetFile string) error {
	os.MkdirAll(c.Network, 0777)

	path := filepath.Join(c.Network, targetFile)

	f, e := os.Create(path)
	if e != nil {
		log.Println("Error creating file: ", e)
		return e
	}

	e = t.Execute(f, c)
	if e != nil {
		log.Println("Error executing template: ", e)
		return e
	}

	return nil
}
