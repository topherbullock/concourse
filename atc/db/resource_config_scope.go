package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"code.cloudfoundry.org/lager"
	sq "github.com/Masterminds/squirrel"
	"github.com/concourse/concourse/atc"
	"github.com/concourse/concourse/atc/db/lock"
)

//go:generate counterfeiter . ResourceConfigScope

// ResourceConfigScope represents the relationship between a possible pipeline resource and a resource config.
// When a resource is specified to have a unique version history either through its base resource type or its custom
// resource type, it results in its generated resource config to be scoped to the resource. This relationship is
// translated into its row in the resource config scopes table to have both the resource id and resource config id
// populated. When a resource has a shared version history, its resource config is not scoped to the (or any) resource
// and its row in the resource config scopes table will have the resource config id populated but a NULL value for
// the resource id. Resource versions will therefore be directly dependent on a resource config scope.
type ResourceConfigScope interface {
	ID() int
	Resource() Resource
	ResourceConfig() ResourceConfig

	SaveVersions(versions []atc.Version) error
	FindVersion(atc.Version) (ResourceConfigVersion, bool, error)
	LatestVersion() (ResourceConfigVersion, bool, error)

	AcquireResourceCheckingLock(
		logger lager.Logger,
		interval time.Duration,
		immediate bool,
	) (lock.Lock, bool, error)
}

type resourceConfigScope struct {
	id             int
	resource       Resource
	resourceConfig ResourceConfig

	conn        Conn
	lockFactory lock.LockFactory
}

func (r *resourceConfigScope) ID() int                        { return r.id }
func (r *resourceConfigScope) Resource() Resource             { return r.resource }
func (r *resourceConfigScope) ResourceConfig() ResourceConfig { return r.resourceConfig }

// SaveVersions stores a list of version in the db for a resource config
// Each version will also have its check order field updated and the
// Cache index for pipelines using the resource config will be bumped.
//
// In the case of a check resource from an older version, the versions
// that already exist in the DB will be re-ordered using
// incrementCheckOrderWhenNewerVersion to input the correct check order
func (r *resourceConfigScope) SaveVersions(versions []atc.Version) error {
	tx, err := r.conn.Begin()
	if err != nil {
		return err
	}

	defer Rollback(tx)

	for _, version := range versions {
		_, err = saveResourceVersion(tx, r, version, nil)
		if err != nil {
			return err
		}

		versionJSON, err := json.Marshal(version)
		if err != nil {
			return err
		}

		err = incrementCheckOrder(tx, r, string(versionJSON))
		if err != nil {
			return err
		}
	}

	err = bumpCacheIndexForPipelinesUsingResourceConfigScope(tx, r)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (r *resourceConfigScope) FindVersion(v atc.Version) (ResourceConfigVersion, bool, error) {
	rcv := &resourceConfigVersion{
		resourceConfigScope: r,
		conn:                r.conn,
	}

	versionByte, err := json.Marshal(v)
	if err != nil {
		return nil, false, err
	}

	row := resourceConfigVersionQuery.
		Where(sq.Eq{
			"v.resource_config_scope_id": r.id,
		}).
		Where(sq.Expr(fmt.Sprintf("v.version_md5 = md5('%s')", versionByte))).
		RunWith(r.conn).
		QueryRow()

	err = scanResourceConfigVersion(rcv, row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, err
	}

	return rcv, true, nil
}

func (r *resourceConfigScope) LatestVersion() (ResourceConfigVersion, bool, error) {
	rcv := &resourceConfigVersion{
		conn:                r.conn,
		resourceConfigScope: r,
	}

	row := resourceConfigVersionQuery.
		Where(sq.Eq{"v.resource_config_scope_id": r.id}).
		OrderBy("v.check_order DESC").
		Limit(1).
		RunWith(r.conn).
		QueryRow()

	err := scanResourceConfigVersion(rcv, row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, err
	}

	return rcv, true, nil
}

func (r *resourceConfigScope) AcquireResourceCheckingLock(
	logger lager.Logger,
	interval time.Duration,
	immediate bool,
) (lock.Lock, bool, error) {
	lock, acquired, err := r.lockFactory.Acquire(
		logger,
		lock.NewResourceConfigCheckingLockID(r.resourceConfig.ID()),
	)
	if err != nil {
		return nil, false, err
	}

	if !acquired {
		return nil, false, nil
	}

	intervalUpdated, err := r.checkIfResourceConfigScopeIntervalUpdated(interval, immediate)
	if err != nil {
		lockErr := lock.Release()
		if lockErr != nil {
			logger.Fatal("failed-to-release-lock", lockErr)
		}
		return nil, false, err
	}

	if !intervalUpdated {
		logger.Debug("failed-to-update-interval", lager.Data{
			"interval":  interval,
			"immediate": immediate,
		})

		lockErr := lock.Release()
		if lockErr != nil {
			logger.Fatal("failed-to-release-lock", lockErr)
		}
		return nil, false, nil
	}

	return lock, true, nil
}

func (r *resourceConfigScope) checkIfResourceConfigScopeIntervalUpdated(
	interval time.Duration,
	immediate bool,
) (bool, error) {
	tx, err := r.conn.Begin()
	if err != nil {
		return false, err
	}

	defer Rollback(tx)

	params := []interface{}{r.id}

	condition := ""
	if !immediate {
		condition = "AND now() - last_checked > ($2 || ' SECONDS')::INTERVAL"
		params = append(params, interval.Seconds())
	}

	updated, err := checkIfRowsUpdated(tx, `
			UPDATE resource_config_scopes
			SET last_checked = now()
			WHERE id = $1
		`+condition, params...)
	if err != nil {
		return false, err
	}

	if !updated {
		return false, nil
	}

	err = tx.Commit()
	if err != nil {
		return false, err
	}

	return true, nil
}

func saveResourceVersion(tx Tx, r ResourceConfigScope, version atc.Version, metadata ResourceConfigMetadataFields) (bool, error) {
	versionJSON, err := json.Marshal(version)
	if err != nil {
		return false, err
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return false, err
	}

	var checkOrder int
	err = tx.QueryRow(`
		INSERT INTO resource_config_versions (resource_config_scope_id, version, version_md5, metadata)
		SELECT $1, $2, md5($3), $4
		ON CONFLICT (resource_config_scope_id, version_md5) DO UPDATE SET metadata = $4
		RETURNING check_order
		`, r.ID(), string(versionJSON), string(versionJSON), string(metadataJSON)).Scan(&checkOrder)
	if err != nil {
		return false, err
	}

	return checkOrder == 0, nil
}

// increment the check order if the version's check order is less than the
// current max. This will fix the case of a check from an old version causing
// the desired order to change; existing versions will be re-ordered since
// we add them in the desired order.
func incrementCheckOrder(tx Tx, r ResourceConfigScope, version string) error {
	_, err := tx.Exec(`
		WITH max_checkorder AS (
			SELECT max(check_order) co
			FROM resource_config_versions
			WHERE resource_config_scope_id = $1
		)

		UPDATE resource_config_versions
		SET check_order = mc.co + 1
		FROM max_checkorder mc
		WHERE resource_config_scope_id = $1
		AND version = $2
		AND check_order <= mc.co;`, r.ID(), version)
	return err
}

func bumpCacheIndexForPipelinesUsingResourceConfigScope(tx Tx, resourceConfigScope ResourceConfigScope) error {
	_, err := tx.Exec(`
		UPDATE pipelines p
		SET cache_index = cache_index + 1
		FROM resources r
		WHERE r.pipeline_id = p.id
		AND r.resource_config_scope_id = $1
	`, resourceConfigScope.ID())
	return err
}
