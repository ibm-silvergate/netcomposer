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
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/ibm-silvergate/netcomposer/netModel"
	"github.com/ibm-silvergate/netcomposer/netSpec"
)

//Flags
var (
	specFile      string
	templatesPath string
	toolsPath     string
	outputPath    string
)

//Paths
var (
	networkPath       string
	volumesPath       string
	cryptoConfigPath  string
	chaincodesPath    string
	genesisPath       string
	channelsPath      string
	networkConfigPath string
)

func readFlags() {
	flag.StringVar(&specFile, "spec", "", "spec file e.g. samplenet.yaml")
	flag.StringVar(&templatesPath, "templates", "templates", "templates path e.g. ./templates")
	flag.StringVar(&toolsPath, "tools", "tools", "tools path e.g. ./tools")
	flag.StringVar(&outputPath, "output", ".", "tools path e.g. $HOME/HF-networks")
	flag.Parse()

	if specFile == "" {
		fmt.Fprintln(os.Stderr, "spec file must be specified")
		os.Exit(1)
	}
}

func main() {

	readFlags()

	netSpec, err := netSpec.LoadFromFile(specFile)
	if err != nil {
		log.Fatalf("Error loading network spec file: %v", err)
		os.Exit(1)
	}

	netSpec.SetDefaults()

	err = netSpec.Validate()

	if err != nil {
		log.Fatalf("Network spec is NOT valid: %v", err)
	}

	netModel := netModel.BuildNetModelFrom(netSpec)

	createPaths(netModel)

	copyChaincodes(netSpec)

	genCryptoConfigFile(netSpec)

	genCryptoMaterial(netModel, "crypto-config.yaml")

	genConfigTXFile(netModel)

	genDockerComposeFile(netModel)

	genPullImagesScriptFile(netModel)

	genNetworkConfigFile(netModel)

	genNetworkConfigForOrgs(netModel)

	genGenesisBlock(netModel, genesisPath, "genesis.block")

	genChannelConfig(netModel, channelsPath)
}

func createPaths(netModel *netModel.NetModel) {
	networkPath = filepath.Join(outputPath, netModel.Name)
	volumesPath = filepath.Join(networkPath, "volumes")
	cryptoConfigPath = filepath.Join(volumesPath, "crypto-config")
	chaincodesPath = filepath.Join(volumesPath, "chaincodes")
	genesisPath = filepath.Join(cryptoConfigPath, "genesis")
	channelsPath = filepath.Join(cryptoConfigPath, "channel-artifacts")
	networkConfigPath = filepath.Join(volumesPath, "network")

	os.MkdirAll(networkPath, 0777)
	os.MkdirAll(volumesPath, 0777)
	os.MkdirAll(cryptoConfigPath, 0777)
	os.MkdirAll(volumesPath, 0777)
	os.MkdirAll(chaincodesPath, 0777)
	os.MkdirAll(genesisPath, 0777)
	os.MkdirAll(channelsPath, 0777)
	os.MkdirAll(networkConfigPath, 0777)
}

func genCryptoConfigFile(spec *netSpec.NetSpec) {
	fmt.Print("Generating crypto config file: ")
	cryptoConfigTemplate := loadTemplate("crypto-config-template.yaml")
	execTemplate(cryptoConfigTemplate, spec, spec.Network, "crypto-config.yaml")
	fmt.Println("SUCCEED")
}

func genConfigTXFile(netModel *netModel.NetModel) {
	fmt.Print("Generating configTX file: ")
	configTXTemplate := loadTemplate("configtx-template.yaml")
	execTemplate(configTXTemplate, netModel, netModel.Name, "configtx.yaml")
	fmt.Println("SUCCEED")
}

func genDockerComposeFile(netModel *netModel.NetModel) {
	fmt.Print("Generating docker compose file: ")
	dockerComposeTemplate := loadTemplate("docker-compose-template.yaml")
	execTemplate(dockerComposeTemplate, netModel, netModel.Name, "docker-compose.yaml")
	fmt.Println("SUCCEED")
}

func genNetworkConfigFile(netModel *netModel.NetModel) {
	fmt.Print("Generating network config file: ")
	networkConfigTemplate := loadTemplate("network-config-template.yaml")
	execTemplate(networkConfigTemplate, netModel, networkConfigPath, "network-config.yaml")
	fmt.Println("SUCCEED")
}

func genNetworkConfigForOrgs(netModel *netModel.NetModel) {
	networkConfigTemplate := loadTemplate("network-config-org-template.yaml")

	for _, org := range netModel.PeerOrganizations {
		fmt.Printf("Generating network config for organization %s: ", org.Name)
		netClientDef := struct {
			Network      string
			Description  string
			Organization string
		}{
			Network:      netModel.Name,
			Description:  netModel.Description,
			Organization: org.Name,
		}

		execTemplate(networkConfigTemplate, netClientDef, networkConfigPath, fmt.Sprintf("network-config-%s.yaml", org.Name))
		fmt.Println("SUCCEED")
	}
}

func genPullImagesScriptFile(netModel *netModel.NetModel) {
	fmt.Print("Generating script to pull fabric docker images: ")
	pullImagesTemplate := loadTemplate("pull-docker-images-template.yaml")
	execTemplate(pullImagesTemplate, netModel, netModel.Name, "pull-docker-images.sh")
	args := []string{"+x", filepath.Join(filepath.Join(outputPath, netModel.Name), "pull-docker-images.sh")}
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

func genCryptoMaterial(netModel *netModel.NetModel, cryptoConfigFile string) {
	fmt.Print("Generating crypto material: ")
	cryptoConfigFilePath := filepath.Join(filepath.Join(outputPath, netModel.Name), cryptoConfigFile)

	args := []string{
		"generate",
		"--config", cryptoConfigFilePath,
		"--output", cryptoConfigPath,
	}

	if err := exec.Command(fmt.Sprintf("%s/%s/cryptogen", toolsPath, architecture()), args...).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	/* Fix naming from cryptogen tool
	* Rename private key files ending in "_sk" to "secret.key" for easier configuration in templates
	 */
	filepath.Walk(filepath.Join(volumesPath, "crypto-config"), fixSKFilename)

	fmt.Println("SUCCEED")
}

func genGenesisBlock(netModel *netModel.NetModel, genesisPath, genesisFile string) {
	fmt.Print("Generating genesis block: ")

	args := []string{
		"-profile", netModel.Name + "Genesis",
		"-outputBlock", filepath.Join(genesisPath, genesisFile),
	}

	cmd := exec.Command(fmt.Sprintf("%s/%s/configtxgen", toolsPath, architecture()), args...)

	netPath, _ := filepath.Abs(networkPath)
	cmd.Env = append(cmd.Env, fmt.Sprintf("FABRIC_CFG_PATH=%s", netPath))

	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("SUCCEED")
}

func genChannelConfig(netModel *netModel.NetModel, channelsPath string) {
	for _, ch := range netModel.Channels {
		fmt.Printf("Generating config for channel %s: ", ch.Name)

		args := []string{
			"-profile", ch.Name,
			"-outputCreateChannelTx", filepath.Join(channelsPath, fmt.Sprintf("%s.tx", ch.Name)),
			"-channelID", ch.Name,
		}

		cmd := exec.Command(fmt.Sprintf("%s/%s/configtxgen", toolsPath, architecture()), args...)

		netPath, _ := filepath.Abs(networkPath)
		cmd.Env = append(cmd.Env, fmt.Sprintf("FABRIC_CFG_PATH=%s", netPath))

		if err := cmd.Run(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("SUCCEED")
	}
}

func copyChaincodes(spec *netSpec.NetSpec) {
	if spec.ChaincodesPath != "" {
		fmt.Printf("Copying chaincodes to %s: ", chaincodesPath)
		copyFolder(spec.ChaincodesPath, chaincodesPath)
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
	templateFilePath := path.Join(templatesPath, templateFile)

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

func execTemplate(t *template.Template, model interface{}, targetPath string, targetFile string) error {
	path := filepath.Join(targetPath, targetFile)

	f, e := os.Create(filepath.Join(outputPath, path))
	if e != nil {
		log.Println("Error creating file: ", e)
		return e
	}

	e = t.Execute(f, model)
	if e != nil {
		log.Println("Error executing template: ", e)
		return e
	}

	return nil
}
