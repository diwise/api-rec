package database

import (
	"context"
	"fmt"

	"github.com/diwise/service-chassis/pkg/infrastructure/env"
	"github.com/diwise/service-chassis/pkg/infrastructure/o11y/logging"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	host     string
	user     string
	password string
	port     string
	dbname   string
	sslmode  string
}

type Database interface {
	AddEntity(ctx context.Context, e Entity) error
	GetEntity(ctx context.Context, entityID, entityType string) (Entity, error)
	GetChildEntities(ctx context.Context, root Entity, entityType string) ([]Entity, error)
}

type databaseImpl struct {
	pool *pgxpool.Pool
}

func LoadConfiguration(ctx context.Context) Config {
	log := logging.GetFromContext(ctx)

	return Config{
		host:     env.GetVariableOrDefault(log, "POSTGRES_HOST", ""),
		user:     env.GetVariableOrDefault(log, "POSTGRES_USER", ""),
		password: env.GetVariableOrDefault(log, "POSTGRES_PASSWORD", ""),
		port:     env.GetVariableOrDefault(log, "POSTGRES_PORT", "5432"),
		dbname:   env.GetVariableOrDefault(log, "POSTGRES_DBNAME", "diwise"),
		sslmode:  env.GetVariableOrDefault(log, "POSTGRES_SSLMODE", "disable"),
	}
}

func (c Config) ConnStr() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", c.user, c.password, c.host, c.port, c.dbname, c.sslmode)
}

func Connect(ctx context.Context, cfg Config) (Database, error) {
	conn, err := pgxpool.New(ctx, cfg.ConnStr())
	if err != nil {
		return nil, err
	}

	err = conn.Ping(ctx)
	if err != nil {
		return nil, err
	}

	log := logging.GetFromContext(ctx)
	log.Debug().Msgf("connected to %s", cfg.host)

	db := databaseImpl{
		pool: conn,
	}

	return &db, nil
}

func (db *databaseImpl) getNodeID(ctx context.Context, entityID, entityType string) (int64, error) {
	row := db.pool.QueryRow(ctx, "SELECT node_id FROM entity WHERE entity_id = $1 AND entity_type = $2", entityID, entityType)
	var nodeId int64 = 0
	err := row.Scan(&nodeId)
	return nodeId, err
}

func (db *databaseImpl) AddEntity(ctx context.Context, e Entity) error {
	_, err := db.pool.Exec(ctx, "INSERT INTO entity (entity_id, entity_type, entity_context) VALUES ($1, $2, $3)", e.Id, e.Type, e.Context)
	if err != nil {
		return err
	}

	if e.IsPartOf == nil {
		return nil
	}

	nodeId, err := db.getNodeID(ctx, e.Id, e.Type)
	if err != nil {
		return err
	}
	partOfNodeId, err := db.getNodeID(ctx, e.IsPartOf.Id, e.IsPartOf.Type)
	if err != nil {
		return err
	}

	err = db.addRelation(ctx, partOfNodeId, nodeId)
	if err != nil {
		return err
	}

	return nil
}

func (db *databaseImpl) addRelation(ctx context.Context, parent, child int64) error {
	_, err := db.pool.Exec(ctx, "INSERT INTO relation (parent, child) VALUES ($1, $2) ON CONFLICT DO NOTHING", parent, child)
	return err
}

func (db *databaseImpl) getParentEntity(ctx context.Context, nodeId int64) (Entity, error) {
	var parentId int64
	relRow := db.pool.QueryRow(ctx, "SELECT parent FROM relation WHERE child = $1", nodeId)
	err := relRow.Scan(&parentId)
	if err != nil {
		return Entity{}, err
	}

	var entityId_, entityType_ string
	entRow := db.pool.QueryRow(ctx, "SELECT entity_id, entity_type FROM entity WHERE node_id = $1", parentId)
	err = entRow.Scan(&entityId_, &entityType_)
	if err != nil {
		return Entity{}, err
	}

	return Entity{
		Id:   entityId_,
		Type: entityType_,
	}, nil
}

func (db *databaseImpl) GetChildEntities(ctx context.Context, root Entity, entityType string) ([]Entity, error) {
	rows, err := db.pool.Query(ctx, `
		WITH RECURSIVE traverse(node_id, entity_type, entity_id) AS (
			SELECT
				node_id,
				entity_type,
				entity_id
			FROM
				entity
			WHERE
				entity.entity_id = $1 AND
				entity.entity_type = $2
			UNION ALL
			SELECT
				entity.node_id,
				entity.entity_type,
				entity.entity_id
			FROM traverse JOIN
			relation ON traverse.node_id = relation.parent JOIN
			entity ON relation.child = entity.node_id
		)
		SELECT
			traverse.entity_id
		FROM traverse
		WHERE traverse.entity_type = $3
		GROUP BY traverse.entity_id
		ORDER BY traverse.entity_id ASC`, root.Id, root.Type, entityType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entities := make([]Entity, 0)

	for rows.Next() {
		var entityId string
		err := rows.Scan(&entityId)
		if err != nil {
			return nil, err
		}
		e, err := db.GetEntity(ctx, entityId, entityType)
		if err != nil {
			return nil, err
		}
		entities = append(entities, e)
	}

	return entities, nil
}

func (db *databaseImpl) GetEntity(ctx context.Context, entityID, entityType string) (Entity, error) {
	var nodeId_ int64
	var entityId_, entityType_, entityContext_ string

	row := db.pool.QueryRow(ctx, "SELECT node_id, entity_id, entity_type, entity_context FROM entity WHERE entity_id = $1 AND entity_type = $2", entityID, entityType)

	err := row.Scan(&nodeId_, &entityId_, &entityType_, &entityContext_)
	if err != nil {
		return Entity{}, err
	}

	e := Entity{
		Context: entityContext_,
		Id:      entityId_,
		Type:    entityType_,
	}

	parent, err := db.getParentEntity(ctx, nodeId_)
	if err == nil {
		e.IsPartOf = &Property{
			Id:   parent.Id,
			Type: parent.Type,
		}
	}

	return e, nil
}

/*

CREATE TYPE entity_type AS ENUM (
    'dtmi:org:w3id:rec:Space;1',
	'dtmi:org:w3id:rec:Building;1',
	'dtmi:org:brickschema:schema:Brick:Sensor;1',
	'dtmi:org:w3id:rec:ObservationEvent;1'
);

CREATE TYPE entity_context AS ENUM (
	'https://dev.realestatecore.io/contexts/Space.jsonld',
	'https://dev.realestatecore.io/contexts/Building.jsonld',
	'https://dev.realestatecore.io/contexts/Sensor.jsonld',
	'https://dev.realestatecore.io/contexts/ObservationEvent.jsonld'
);

CREATE TABLE entity (
    node_id        BIGSERIAL,
	entity_id      TEXT NOT NULL,
    entity_type    entity_type NOT NULL,
	entity_context entity_context NOT NULL,
    PRIMARY KEY (node_id)
);

CREATE UNIQUE INDEX entity_entity_type_entity_id_unique_indx ON entity (entity_type, entity_id);

CREATE table relation (
    parent        BIGINT NOT NULL,
    child         BIGINT NOT NULL,
    PRIMARY KEY (parent, child)
);

CREATE INDEX relation_child_parent_indx ON relation(child, parent);

*/
