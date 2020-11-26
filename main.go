package main

import (
	"context"
	"fmt"
	"log"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	uuid "github.com/satori/go.uuid"
)

func ifErr(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	dockerClient, err := client.NewEnvClient()
	ifErr(err)

	cleanupLabels := map[string]string{
		uuid.NewV4().String(): uuid.NewV4().String(),
	}

	// create networks
	networkNamesAndAliases := map[string][]string{
		uuid.NewV4().String(): {uuid.NewV4().String()},
		uuid.NewV4().String(): {uuid.NewV4().String()},
	}
	networkIDs := make([]string, 0)
	networkNames := make([]string, 0)
	networkMapping := make(map[string]string)

	ctx := context.Background()
	for networkName := range networkNamesAndAliases {
		networkCreateResp, err := dockerClient.NetworkCreate(ctx, networkName, types.NetworkCreate{
			Labels: cleanupLabels,
		})
		ifErr(err)
		networkIDs = append(networkIDs, networkCreateResp.ID)
		networkNames = append(networkNames, networkName)
		networkMapping[networkName] = networkCreateResp.ID
	}

	// cleanup docker networks
	defer func() {
		for i := range networkIDs {
			dockerClient.NetworkRemove(ctx, networkIDs[i])
		}
	}()

	// prepare networkConfig
	networkConfig := network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			networkNames[0]: {
				Aliases:   networkNamesAndAliases[networkNames[0]],
				NetworkID: networkMapping[networkNames[0]],
			},
		},
	}

	// create container
	containerName := uuid.NewV4().String()
	containerCreateResp, err := dockerClient.ContainerCreate(ctx,
		&container.Config{
			Image:  "docker.io/library/nginx:latest",
			Labels: cleanupLabels,
		},
		&container.HostConfig{},
		&networkConfig,
		containerName)
	ifErr(err)

	// cleanup container
	defer func() {
		dockerClient.ContainerRemove(ctx, containerCreateResp.ID, types.ContainerRemoveOptions{
			Force: true,
		})
	}()

	// add additional network
	err = dockerClient.NetworkConnect(ctx, networkIDs[1], containerCreateResp.ID, &network.EndpointSettings{
		Aliases:   networkNamesAndAliases[networkNames[1]],
		NetworkID: networkMapping[networkNames[1]],
	})
	ifErr(err)

	// get containers by name
	containerNetworks, err := networkListByNames(ctx, dockerClient, networkNames...)
	ifErr(err)

	// test if container is part of the defined networks
	for i := range containerNetworks {
		if len(containerNetworks[i].Containers) != 1 {
			log.Printf("Container %v is not part of network %v", containerCreateResp.ID, containerNetworks[i].ID)
		}
	}

}

func networkListByNames(ctx context.Context, dockerClient *client.Client, networkNames ...string) ([]types.NetworkResource, error) {
	networks, err := dockerClient.NetworkList(ctx, types.NetworkListOptions{})
	if err != nil {
		return nil, err
	}

	foundNetwork := make(map[string]bool, 0)
	for _, networkName := range networkNames {
		foundNetwork[networkName] = false
	}

	filteredNetworks := make([]types.NetworkResource, 0)
	for _, networkName := range networkNames {
		for _, network := range networks {
			if network.Name == networkName {
				filteredNetworks = append(filteredNetworks, network)
				foundNetwork[networkName] = true
				break
			}
		}
	}

	for _, networkName := range networkNames {
		if !foundNetwork[networkName] {
			return nil, fmt.Errorf("Error searching for network %v: Network not found", networkName)
		}
	}

	return filteredNetworks, nil
}
