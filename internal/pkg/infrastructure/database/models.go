package database

import (
	"strings"
	"time"
)

type Property struct {
	Id   string `json:"@id"`
	Type string `json:"@type"`
}

type Entity struct {
	Context  string    `json:"@context"`
	Id       string    `json:"@id"`
	Type     string    `json:"@type"`
	IsPartOf *Property `json:"isPartOf,omitempty"`
}

type SensorObservation struct {
	Format       string        `json:"format"`
	DeviceID     string        `json:"deviceId"`
	Observations []Observation `json:"observations"`
}

type Observation struct {
	ObservationTime time.Time `json:"observationTime"`
	Value           *float64  `json:"value,omitempty"`
	ValueString     *string   `json:"valueString,omitempty"`
	ValueBoolean    *bool     `json:"valueBoolean,omitempty"`
	QuantityKind    string    `json:"quantityKind"`
	SensorId        string    `json:"sensorId"`
}

const (
	SpaceContext             string = "https://dev.realestatecore.io/contexts/Space.jsonld"
	SpaceType                string = "dtmi:org:w3id:rec:Space;1"
	SpaceTypeName            string = "space"
	BuildingContext          string = "https://dev.realestatecore.io/contexts/Building.jsonld"
	BuildingType             string = "dtmi:org:w3id:rec:Building;1"
	BuildingTypeName         string = "building"
	SensorContext            string = "https://dev.realestatecore.io/contexts/Sensor.jsonld"
	SensorType               string = "dtmi:org:brickschema:schema:Brick:Sensor;1"
	SensorTypeName           string = "sensor"
	ObservationEventContext  string = "https://dev.realestatecore.io/contexts/ObservationEvent.jsonld"
	ObservationEventType     string = "dtmi:org:w3id:rec:ObservationEvent;1"
	ObservationEventTypeName string = "observationevent"
)

func GetTypeFromTypeName(typeName string) string {
	switch strings.ToLower(typeName) {
	case SpaceTypeName:
		return SpaceType
	case BuildingTypeName:
		return BuildingType
	case SensorTypeName:
		return SensorType
	case ObservationEventTypeName:
		return ObservationEventType
	}
	return ""
}
