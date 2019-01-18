package containerserver

import (
	"github.com/concourse/concourse/atc"
	"github.com/concourse/concourse/atc/creds"
	"github.com/concourse/concourse/atc/creds/noop"
	"github.com/concourse/concourse/atc/worker"
	"io"
	"io/ioutil"
	"net/http"

	"code.cloudfoundry.org/lager"
	"github.com/concourse/concourse/atc/db"
)

func (s *Server) CreateShell(team db.Team) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		params := r.URL.RawQuery
		hLog := s.logger.Session("create-shell", lager.Data{
			"params": params,
		})

		s.workerClient.FindOrCreateContainer(r.Context(), hLog, &imageFetchingDelegate{},
			db.ShellContainerOwner{
				TeamID:team.ID(),
			},
			db.ContainerMetadata{
				Type: db.ContainerTypeShell,
			},
			worker.ContainerSpec{
				Platform: "linux",
				TeamID:team.ID(),
				ImageSpec: worker.ImageSpec{
					ImageResource: &worker.ImageResource{
						Type:"registry-image",
						Source: creds.NewSource(noop.Noop{}, atc.Source{ "repository" : "alpine"}),
					},
				},
			},
			worker.WorkerSpec{
				Platform: "linux",
				ResourceType:"registry-image",
				TeamID: team.ID(),
			},
			creds.VersionedResourceTypes{}, // does this matter??
		)
	})
}

type imageFetchingDelegate struct{}

func (*imageFetchingDelegate) Stdout() io.Writer {
	return ioutil.Discard
}

func (*imageFetchingDelegate) Stderr() io.Writer {
	return ioutil.Discard
}

func (*imageFetchingDelegate) ImageVersionDetermined(cache db.UsedResourceCache) error {
	return nil
}


