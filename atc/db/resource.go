package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"code.cloudfoundry.org/lager"
	sq "github.com/Masterminds/squirrel"
	"github.com/concourse/concourse/atc"
	"github.com/concourse/concourse/atc/creds"
	"github.com/concourse/concourse/atc/db/lock"
	"github.com/lib/pq"
)

//go:generate counterfeiter . Resource

type Resource interface {
	ID() int
	Name() string
	PipelineID() int
	PipelineName() string
	TeamName() string
	Type() string
	Source() atc.Source
	CheckEvery() string
	CheckTimeout() string
	LastChecked() time.Time
	Tags() atc.Tags
	CheckError() error
	WebhookToken() string
	ConfigPinnedVersion() atc.Version
	APIPinnedVersion() atc.Version
	ResourceConfigCheckError() error
	ResourceConfigID() int
	UniqueVersionHistory() bool

	CurrentPinnedVersion() atc.Version

	ResourceConfigVersionID(atc.Version) (int, bool, error)
	Versions(page Page) ([]atc.ResourceVersion, Pagination, bool, error)

	EnableVersion(rcvID int) error
	DisableVersion(rcvID int) error

	PinVersion(rcvID int) error
	UnpinVersion() error

	SetResourceConfig(lager.Logger, atc.Source, creds.VersionedResourceTypes) (ResourceConfig, error)
	SetCheckError(error) error

	Reload() (bool, error)
}

var resourcesQuery = psql.Select("r.id, r.name, r.config, r.check_error, c.last_checked, r.pipeline_id, r.nonce, r.resource_config_id, p.name, t.name, c.check_error, r.api_pinned_version").
	From("resources r").
	Join("pipelines p ON p.id = r.pipeline_id").
	Join("teams t ON t.id = p.team_id").
	LeftJoin("resource_configs c ON r.resource_config_id = c.id").
	Where(sq.Eq{"r.active": true})

type resource struct {
	id                       int
	name                     string
	pipelineID               int
	pipelineName             string
	teamName                 string
	type_                    string
	source                   atc.Source
	checkEvery               string
	checkTimeout             string
	lastChecked              time.Time
	tags                     atc.Tags
	checkError               error
	webhookToken             string
	configPinnedVersion      atc.Version
	apiPinnedVersion         atc.Version
	resourceConfigCheckError error
	resourceConfigID         int
	uniqueVersionHistory     bool

	conn        Conn
	lockFactory lock.LockFactory
}

type ResourceNotFoundError struct {
	Name string
}

func (e ResourceNotFoundError) Error() string {
	return fmt.Sprintf("resource '%s' not found", e.Name)
}

type Resources []Resource

func (resources Resources) Lookup(name string) (Resource, bool) {
	for _, resource := range resources {
		if resource.Name() == name {
			return resource, true
		}
	}

	return nil, false
}

func (resources Resources) Configs() atc.ResourceConfigs {
	var configs atc.ResourceConfigs

	for _, r := range resources {
		configs = append(configs, atc.ResourceConfig{
			Name:         r.Name(),
			WebhookToken: r.WebhookToken(),
			Type:         r.Type(),
			Source:       r.Source(),
			CheckEvery:   r.CheckEvery(),
			Tags:         r.Tags(),
			Version:      r.ConfigPinnedVersion(),
		})
	}

	return configs
}

func (r *resource) ID() int                          { return r.id }
func (r *resource) Name() string                     { return r.name }
func (r *resource) PipelineID() int                  { return r.pipelineID }
func (r *resource) PipelineName() string             { return r.pipelineName }
func (r *resource) TeamName() string                 { return r.teamName }
func (r *resource) Type() string                     { return r.type_ }
func (r *resource) Source() atc.Source               { return r.source }
func (r *resource) CheckEvery() string               { return r.checkEvery }
func (r *resource) CheckTimeout() string             { return r.checkTimeout }
func (r *resource) LastChecked() time.Time           { return r.lastChecked }
func (r *resource) Tags() atc.Tags                   { return r.tags }
func (r *resource) CheckError() error                { return r.checkError }
func (r *resource) WebhookToken() string             { return r.webhookToken }
func (r *resource) ConfigPinnedVersion() atc.Version { return r.configPinnedVersion }
func (r *resource) APIPinnedVersion() atc.Version    { return r.apiPinnedVersion }
func (r *resource) ResourceConfigCheckError() error  { return r.resourceConfigCheckError }
func (r *resource) ResourceConfigID() int            { return r.resourceConfigID }
func (r *resource) UniqueVersionHistory() bool       { return r.uniqueVersionHistory }

func (r *resource) Reload() (bool, error) {
	row := resourcesQuery.Where(sq.Eq{"r.id": r.id}).
		RunWith(r.conn).
		QueryRow()

	err := scanResource(r, row)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func (r *resource) SetResourceConfig(logger lager.Logger, source atc.Source, resourceTypes creds.VersionedResourceTypes) (ResourceConfig, error) {
	resourceConfigDescriptor, err := constructResourceConfigDescriptor(r.type_, source, resourceTypes, r)
	if err != nil {
		return nil, err
	}

	tx, err := r.conn.Begin()
	if err != nil {
		return nil, err
	}

	defer Rollback(tx)

	resourceConfig, err := resourceConfigDescriptor.findOrCreate(logger, tx, r.lockFactory, r.conn)
	if err != nil {
		return nil, err
	}

	results, err := psql.Update("resources").
		Set("resource_config_id", resourceConfig.ID()).
		Where(sq.Eq{"id": r.id}).
		Where(sq.Or{
			sq.Eq{"resource_config_id": nil},
			sq.NotEq{"resource_config_id": resourceConfig.ID()},
		}).
		RunWith(tx).
		Exec()
	if err != nil {
		return nil, err
	}

	rowsAffected, err := results.RowsAffected()
	if err != nil {
		return nil, err
	}

	if rowsAffected > 0 {
		err = bumpCacheIndexForPipelinesUsingResourceConfig(tx, resourceConfig.ID())
		if err != nil {
			return nil, err
		}
	}

	err = tx.Commit()
	if err != nil {
		return nil, err
	}

	return resourceConfig, nil
}

func (r *resource) SetCheckError(cause error) error {
	var err error

	if cause == nil {
		_, err = psql.Update("resources").
			Set("check_error", nil).
			Where(sq.Eq{"id": r.ID()}).
			RunWith(r.conn).
			Exec()
	} else {
		_, err = psql.Update("resources").
			Set("check_error", cause.Error()).
			Where(sq.Eq{"id": r.ID()}).
			RunWith(r.conn).
			Exec()
	}

	return err
}

func (r *resource) ResourceConfigVersionID(version atc.Version) (int, bool, error) {
	requestedVersion, err := json.Marshal(version)
	if err != nil {
		return 0, false, err
	}

	var id int
	err = psql.Select("rcv.id").
		From("resource_config_versions rcv").
		Join("resource_configs rc ON rc.id = resource_config_id").
		Join("resources r ON rc.id = r.resource_config_id").
		Where(sq.Eq{"r.id": r.ID(), "version": requestedVersion}).
		Where(sq.NotEq{"rcv.check_order": 0}).
		RunWith(r.conn).
		QueryRow().
		Scan(&id)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, false, nil
		}
		return 0, false, err
	}

	return id, true, nil
}

func (r *resource) CurrentPinnedVersion() atc.Version {
	if r.configPinnedVersion != nil {
		return r.configPinnedVersion
	} else if r.apiPinnedVersion != nil {
		return r.apiPinnedVersion
	}
	return nil
}

func (r *resource) Versions(page Page) ([]atc.ResourceVersion, Pagination, bool, error) {
	query := `
		SELECT v.id, v.version, v.metadata, v.check_order,
			NOT EXISTS (
				SELECT 1
				FROM resource_disabled_versions d
				WHERE v.version_md5 = d.version_md5
				AND r.resource_config_id = v.resource_config_id
				AND r.id = d.resource_id
			)
		FROM resource_config_versions v, resources r
		WHERE r.id = $1 AND r.resource_config_id = v.resource_config_id AND v.check_order != 0
	`

	var rows *sql.Rows
	var err error
	if page.Until != 0 {
		rows, err = r.conn.Query(fmt.Sprintf(`
			SELECT sub.*
				FROM (
						%s
					AND v.check_order > (SELECT check_order FROM resource_config_versions WHERE id = $2)
				ORDER BY v.check_order ASC
				LIMIT $3
			) sub
			ORDER BY sub.check_order DESC
		`, query), r.id, page.Until, page.Limit)
		if err != nil {
			return nil, Pagination{}, false, err
		}
	} else if page.Since != 0 {
		rows, err = r.conn.Query(fmt.Sprintf(`
			%s
				AND v.check_order < (SELECT check_order FROM resource_config_versions WHERE id = $2)
			ORDER BY v.check_order DESC
			LIMIT $3
		`, query), r.id, page.Since, page.Limit)
		if err != nil {
			return nil, Pagination{}, false, err
		}
	} else if page.To != 0 {
		rows, err = r.conn.Query(fmt.Sprintf(`
			SELECT sub.*
				FROM (
						%s
					AND v.check_order >= (SELECT check_order FROM resource_config_versions WHERE id = $2)
				ORDER BY v.check_order ASC
				LIMIT $3
			) sub
			ORDER BY sub.check_order DESC
		`, query), r.id, page.To, page.Limit)
		if err != nil {
			return nil, Pagination{}, false, err
		}
	} else if page.From != 0 {
		rows, err = r.conn.Query(fmt.Sprintf(`
			%s
				AND v.check_order <= (SELECT check_order FROM resource_config_versions WHERE id = $2)
			ORDER BY v.check_order DESC
			LIMIT $3
		`, query), r.id, page.From, page.Limit)
		if err != nil {
			return nil, Pagination{}, false, err
		}
	} else {
		rows, err = r.conn.Query(fmt.Sprintf(`
			%s
			ORDER BY v.check_order DESC
			LIMIT $2
		`, query), r.id, page.Limit)
		if err != nil {
			return nil, Pagination{}, false, err
		}
	}

	defer Close(rows)

	type rcvCheckOrder struct {
		ResourceConfigVersionID int
		CheckOrder              int
	}

	rvs := make([]atc.ResourceVersion, 0)
	checkOrderRVs := make([]rcvCheckOrder, 0)
	for rows.Next() {
		var (
			metadataBytes sql.NullString
			versionBytes  string
			checkOrder    int
		)

		rv := atc.ResourceVersion{}
		err := rows.Scan(&rv.ID, &versionBytes, &metadataBytes, &checkOrder, &rv.Enabled)
		if err != nil {
			return nil, Pagination{}, false, err
		}

		err = json.Unmarshal([]byte(versionBytes), &rv.Version)
		if err != nil {
			return nil, Pagination{}, false, err
		}

		if metadataBytes.Valid {
			err = json.Unmarshal([]byte(metadataBytes.String), &rv.Metadata)
			if err != nil {
				return nil, Pagination{}, false, err
			}
		}

		checkOrderRV := rcvCheckOrder{
			ResourceConfigVersionID: rv.ID,
			CheckOrder:              checkOrder,
		}

		rvs = append(rvs, rv)
		checkOrderRVs = append(checkOrderRVs, checkOrderRV)
	}

	if len(rvs) == 0 {
		return nil, Pagination{}, true, nil
	}

	var minCheckOrder int
	var maxCheckOrder int

	err = r.conn.QueryRow(`
		SELECT COALESCE(MAX(v.check_order), 0) as maxCheckOrder,
			COALESCE(MIN(v.check_order), 0) as minCheckOrder
		FROM resource_config_versions v, resources r
		WHERE r.id = $1 AND v.resource_config_id = r.resource_config_id
	`, r.id).Scan(&maxCheckOrder, &minCheckOrder)
	if err != nil {
		return nil, Pagination{}, false, err
	}

	firstRCVCheckOrder := checkOrderRVs[0]
	lastRCVCheckOrder := checkOrderRVs[len(checkOrderRVs)-1]

	var pagination Pagination

	if firstRCVCheckOrder.CheckOrder < maxCheckOrder {
		pagination.Previous = &Page{
			Until: firstRCVCheckOrder.ResourceConfigVersionID,
			Limit: page.Limit,
		}
	}

	if lastRCVCheckOrder.CheckOrder > minCheckOrder {
		pagination.Next = &Page{
			Since: lastRCVCheckOrder.ResourceConfigVersionID,
			Limit: page.Limit,
		}
	}

	return rvs, pagination, true, nil
}

func (r *resource) EnableVersion(rcvID int) error {
	return r.toggleVersion(rcvID, true)
}

func (r *resource) DisableVersion(rcvID int) error {
	return r.toggleVersion(rcvID, false)
}

func (r *resource) PinVersion(rcvID int) error {
	results, err := r.conn.Exec(`
			UPDATE resources SET (api_pinned_version) =
			( SELECT rcv.version
				FROM resource_config_versions rcv
				WHERE rcv.id = $1 )
			WHERE resources.id = $2
			`, rcvID, r.id)
	if err != nil {
		return err
	}

	rowsAffected, err := results.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected != 1 {
		return nonOneRowAffectedError{rowsAffected}
	}

	return nil
}

func (r *resource) UnpinVersion() error {
	results, err := psql.Update("resources").
		Set("api_pinned_version", sq.Expr("NULL")).
		Where(sq.Eq{"resources.id": r.id}).
		RunWith(r.conn).
		Exec()
	if err != nil {
		return err
	}

	rowsAffected, err := results.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected != 1 {
		return nonOneRowAffectedError{rowsAffected}
	}

	return nil
}

func (r *resource) toggleVersion(rcvID int, enable bool) error {
	tx, err := r.conn.Begin()
	if err != nil {
		return err
	}

	defer Rollback(tx)

	var results sql.Result
	if enable {
		results, err = tx.Exec(`
			DELETE FROM resource_disabled_versions
			WHERE resource_id = $1
			AND version_md5 = (SELECT version_md5 FROM resource_config_versions rcv WHERE rcv.id = $2)
			`, r.id, rcvID)
	} else {
		results, err = tx.Exec(`
			INSERT INTO resource_disabled_versions (resource_id, version_md5)
			SELECT $1, rcv.version_md5
			FROM resource_config_versions rcv
			WHERE rcv.id = $2
			`, r.id, rcvID)
	}
	if err != nil {
		return err
	}

	rowsAffected, err := results.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected != 1 {
		return nonOneRowAffectedError{rowsAffected}
	}

	err = bumpCacheIndex(tx, r.pipelineID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func scanResource(r *resource, row scannable) error {
	var (
		configBlob                                          []byte
		checkErr, rcCheckErr, nonce, rcID, apiPinnedVersion sql.NullString
		lastChecked                                         pq.NullTime
	)

	err := row.Scan(&r.id, &r.name, &configBlob, &checkErr, &lastChecked, &r.pipelineID, &nonce, &rcID, &r.pipelineName, &r.teamName, &rcCheckErr, &apiPinnedVersion)
	if err != nil {
		return err
	}

	r.lastChecked = lastChecked.Time

	es := r.conn.EncryptionStrategy()

	var noncense *string
	if nonce.Valid {
		noncense = &nonce.String
	}

	decryptedConfig, err := es.Decrypt(string(configBlob), noncense)
	if err != nil {
		return err
	}

	var config atc.ResourceConfig
	err = json.Unmarshal(decryptedConfig, &config)
	if err != nil {
		return err
	}

	r.type_ = config.Type
	r.source = config.Source
	r.checkEvery = config.CheckEvery
	r.checkTimeout = config.CheckTimeout
	r.tags = config.Tags
	r.webhookToken = config.WebhookToken
	r.configPinnedVersion = config.Version
	r.uniqueVersionHistory = config.UniqueVersionHistory

	if apiPinnedVersion.Valid {
		err = json.Unmarshal([]byte(apiPinnedVersion.String), &r.apiPinnedVersion)
		if err != nil {
			return err
		}
	} else {
		r.apiPinnedVersion = nil
	}

	if checkErr.Valid {
		r.checkError = errors.New(checkErr.String)
	} else {
		r.checkError = nil
	}

	if rcCheckErr.Valid {
		r.resourceConfigCheckError = errors.New(rcCheckErr.String)
	} else {
		r.resourceConfigCheckError = nil
	}

	if rcID.Valid {
		r.resourceConfigID, err = strconv.Atoi(rcID.String)
		if err != nil {
			return err
		}
	}

	return nil
}
