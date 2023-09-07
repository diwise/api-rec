package database

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func connect() (context.Context, context.CancelFunc, Database, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

	db, err := Connect(ctx, Config{
		host:     "localhost",
		user:     "postgres",
		password: "password",
		port:     "5432",
		dbname:   "postgres",
		sslmode:  "disable",
	})
	if err != nil {
		return ctx, cancel, nil, err
	}
	err = db.Init(ctx)
	if err != nil {
		return ctx, cancel, nil, err
	}

	return ctx, cancel, db, nil
}

func TestAddAndGetEntity(t *testing.T) {
	ctx, cancel, db, err := connect()
	defer cancel()

	if err != nil {
		t.Log("could not connect to database or create tables, will skip test")
		t.SkipNow()
	}

	id := uuid.New().String()

	err = db.AddEntity(ctx, Entity{
		Context: BuildingContext,
		Id:      id,
		Type:    BuildingType,
	})
	if err != nil {
		t.FailNow()
	}

	e, err := db.GetEntity(ctx, id, BuildingType)
	if err != nil {
		t.FailNow()
	}

	if e.Id != id {
		t.Fail()
	}
}

func TestGetEntities(t *testing.T) {
	ctx, cancel, db, err := connect()
	defer cancel()

	if err != nil {
		t.Log("could not connect to database or create tables, will skip test")
		t.SkipNow()
	}

	id1 := uuid.New().String()

	err = db.AddEntity(ctx, Entity{
		Context: BuildingContext,
		Id:      id1,
		Type:    BuildingType,
	})
	if err != nil {
		t.FailNow()
	}

	id2 := uuid.New().String()

	err = db.AddEntity(ctx, Entity{
		Context: BuildingContext,
		Id:      id2,
		Type:    BuildingType,
	})
	if err != nil {
		t.FailNow()
	}

	count, e, err := db.GetEntities(ctx, BuildingType, 0, 1000)
	if err != nil {
		t.FailNow()
	}

	if count > 1000 {
		t.Logf("database contains too many entities (%d)", count)
		t.SkipNow()
	}

	if !slices.ContainsFunc(e, func(e Entity) bool {
		return e.Id == id1
	}) {
		t.Fail()
	}
	if !slices.ContainsFunc(e, func(e Entity) bool {
		return e.Id == id2
	}) {
		t.Fail()
	}
}

func TestGetChildEntities(t *testing.T) {
	ctx, cancel, db, err := connect()
	defer cancel()

	if err != nil {
		t.Log("could not connect to database or create tables, will skip test")
		t.SkipNow()
	}

	parentID := uuid.New().String()

	err = db.AddEntity(ctx, Entity{
		Context: BuildingContext,
		Id:      parentID,
		Type:    BuildingType,
	})
	if err != nil {
		t.FailNow()
	}

	childID := uuid.New().String()

	err = db.AddEntity(ctx, Entity{
		Context: SensorContext,
		Id:      childID,
		Type:    SensorType,
		IsPartOf: &Property{
			Id:   parentID,
			Type: BuildingType,
		},
	})
	if err != nil {
		t.FailNow()
	}

	root, err := db.GetEntity(ctx, parentID, BuildingType)
	if err != nil {
		t.FailNow()
	}

	e, err := db.GetChildEntities(ctx, root, SensorType)
	if err != nil {
		t.FailNow()
	}

	if len(e) != 1 {
		t.Log("1 != 1, there sould be only one!")
		t.FailNow()
	}

	if e[0].Id != childID {
		t.Logf("expected %s but got %s", childID, e[0].Id)
		t.FailNow()
	}

	if e[0].IsPartOf.Id != parentID {
		t.Logf("expected %s but got %s", parentID, e[0].IsPartOf.Id)
		t.FailNow()
	}
}

func TestAddAndGetObservations(t *testing.T) {
	ctx, cancel, db, err := connect()
	defer cancel()

	if err != nil {
		t.Log("could not connect to database or create tables, will skip test")
		t.SkipNow()
	}

	now := time.Now().UTC()
	v := 14.123456789
	deviceID := uuid.New().String()
	sensorID := uuid.New().String()

	err = db.AddObservation(ctx, SensorObservation{
		Format:   "ref3.1",
		DeviceID: deviceID,
		Observations: []Observation{
			{
				ObservationTime: now,
				Value:           &v,
				QuantityKind:    "Float",
				SensorId:        sensorID,
			},
		},
	})
	if err != nil {
		t.FailNow()
	}

	// this observation is the same as the previous one,
	// so this should be ignored by code and design without errors
	err = db.AddObservation(ctx, SensorObservation{
		Format:   "ref3.1",
		DeviceID: deviceID,
		Observations: []Observation{
			{
				ObservationTime: now,
				Value:           &v,
				QuantityKind:    "Float",
				SensorId:        sensorID,
			},
		},
	})
	if err != nil {
		t.FailNow()
	}

	vb := true

	err = db.AddObservation(ctx, SensorObservation{
		Format:   "ref3.1",
		DeviceID: deviceID,
		Observations: []Observation{
			{
				ObservationTime: now,
				ValueBoolean:    &vb,
				QuantityKind:    "Bool",
				SensorId:        sensorID,
			},
		},
	})
	if err != nil {
		t.FailNow()
	}

	vs := "string"

	err = db.AddObservation(ctx, SensorObservation{
		Format:   "ref3.1",
		DeviceID: deviceID,
		Observations: []Observation{
			{
				ObservationTime: now,
				ValueString:     &vs,
				QuantityKind:    "String",
				SensorId:        sensorID,
			},
		},
	})
	if err != nil {
		t.FailNow()
	}

	count, _, err := db.GetObservations(ctx, sensorID, now.Add(-5*time.Second), now.Add(1*time.Minute), 0, 5)
	if err != nil {
		t.FailNow()
	}

	if count != 3 {
		t.Logf("%d != 3, should have created 3 observations but found %d", count, count)
		t.Fail()
	}
}

func TestSeed(t *testing.T) {
	ctx, cancel, db, err := connect()
	defer cancel()

	if err != nil {
		t.Log("could not connect to database or create tables, will skip test")
		t.SkipNow()
	}

	spaceID := uuid.New().String()
	sensorID := uuid.New().String()
	buildingID := uuid.New().String()

	csv := fmt.Sprintf(`
spaces;buildings;sensors
%s;%s;%s-1
%s;%s;%s-2`, spaceID, buildingID, sensorID, spaceID, buildingID, sensorID)

	err = db.Seed(ctx, strings.NewReader(csv))
	if err != nil {
		t.FailNow()
	}

	root, err := db.GetEntity(ctx, spaceID, SpaceType)
	if err != nil {
		t.Log("could not find root entity")
		t.FailNow()
	}

	e, err := db.GetChildEntities(ctx, root, SensorType)
	if err != nil {
		t.Log("could not get child entities")
		t.FailNow()
	}

	if len(e) != 2 {
		t.Logf("2 != 2, there sould be two! found %d", len(e))
		t.FailNow()
	}

	if e[0].IsPartOf.Id != buildingID {
		t.Logf("expected %s but got %s", buildingID, e[0].IsPartOf.Id)
		t.FailNow()
	}
}
