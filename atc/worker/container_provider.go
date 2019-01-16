package worker

import (
	"context"
	"path/filepath"
	"time"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/lager"
	"github.com/concourse/concourse/atc/creds"
	"github.com/concourse/concourse/atc/db"
	"github.com/concourse/concourse/atc/db/lock"
)

const creatingContainerRetryDelay = 1 * time.Second

func NewContainerProvider(
	gardenClient garden.Client,
	volumeClient VolumeClient,
	dbWorker db.Worker,
	imageFactory ImageFactory,
	dbVolumeRepository db.VolumeRepository,
	dbTeamFactory db.TeamFactory,
	lockFactory lock.LockFactory,
) ContainerProvider {

	return &containerProvider{
		gardenClient:       gardenClient,
		volumeClient:       volumeClient,
		imageFactory:       imageFactory,
		dbVolumeRepository: dbVolumeRepository,
		dbTeamFactory:      dbTeamFactory,
		lockFactory:        lockFactory,
		httpProxyURL:       dbWorker.HTTPProxyURL(),
		httpsProxyURL:      dbWorker.HTTPSProxyURL(),
		noProxy:            dbWorker.NoProxy(),
		worker:             dbWorker,
	}
}

//go:generate counterfeiter . ContainerProvider

type ContainerProvider interface {
	FindCreatedContainerByHandle(
		logger lager.Logger,
		handle string,
		teamID int,
	) (Container, bool, error)

	FindOrCreateContainer(
		ctx context.Context,
		logger lager.Logger,
		owner db.ContainerOwner,
		delegate ImageFetchingDelegate,
		metadata db.ContainerMetadata,
		containerSpec ContainerSpec,
		workerSpec WorkerSpec,
		resourceTypes creds.VersionedResourceTypes,
		image Image,
	) (Container, error)
}

// TODO: Remove the ImageFactory from the containerProvider.
// Currently, the imageFactory is only needed to create a garden
// worker in createGardenContainer. Creating a garden worker here
// is cyclical because the garden worker contains a containerProvider.
// There is an ongoing refactor that is attempting to fix this.
type containerProvider struct {
	gardenClient       garden.Client
	volumeClient       VolumeClient
	imageFactory       ImageFactory
	dbVolumeRepository db.VolumeRepository
	dbTeamFactory      db.TeamFactory

	lockFactory lock.LockFactory

	worker        db.Worker
	httpProxyURL  string
	httpsProxyURL string
	noProxy       string
}

// If a created container exists, a garden.Container must also exist
// so this method will find it, create the corresponding worker.Container
// and return it.
// If no created container exists, FindOrCreateContainer will go through
// the container creation flow i.e. find or create a CreatingContainer,
// create the garden.Container and then the CreatedContainer
func (p *containerProvider) FindOrCreateContainer(
	ctx context.Context,
	logger lager.Logger,
	owner db.ContainerOwner,
	delegate ImageFetchingDelegate,
	metadata db.ContainerMetadata,
	containerSpec ContainerSpec,
	workerSpec WorkerSpec,
	resourceTypes creds.VersionedResourceTypes,
	image Image,
) (Container, error) {
	return nil, nil
}

func (p *containerProvider) FindCreatedContainerByHandle(
	logger lager.Logger,
	handle string,
	teamID int,
) (Container, bool, error) {
	return nil, false, nil
}



func getDestinationPathsFromInputs(inputs []InputSource) []string {
	destinationPaths := make([]string, len(inputs))

	for idx, input := range inputs {
		destinationPaths[idx] = input.DestinationPath()
	}

	return destinationPaths
}

func getDestinationPathsFromOutputs(outputs OutputPaths) []string {
	var (
		idx              = 0
		destinationPaths = make([]string, len(outputs))
	)

	for _, destinationPath := range outputs {
		destinationPaths[idx] = destinationPath
		idx++
	}

	return destinationPaths
}

func anyMountTo(path string, destinationPaths []string) bool {
	for _, destinationPath := range destinationPaths {
		if filepath.Clean(destinationPath) == filepath.Clean(path) {
			return true
		}
	}

	return false
}
