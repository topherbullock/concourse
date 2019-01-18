package concourse

import (
	"github.com/concourse/concourse/atc"
	"github.com/concourse/concourse/go-concourse/concourse/internal"
	"github.com/tedsuo/rata"
)

func (team *team) CreateShell() (atc.Container, error) {
	var container atc.Container

	params := rata.Params{
		"team_name": team.name,
	}
	err := team.connection.Send(internal.Request{
		RequestName: atc.CreateShell,
		Params:      params,
	}, &internal.Response{
		Result: &container,
	})
	return container, err
}
