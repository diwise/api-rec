package database

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

func NewSpace(id string) Entity {
	return Entity{
		Context: "https://dev.realestatecore.io/contexts/Space.jsonld",
		Type:    "dtmi:org:w3id:rec:Space;1",
		Id:      id,
	}
}

func NewBuilding(id string) Entity {
	return Entity{
		Context: "https://dev.realestatecore.io/contexts/Building.jsonld",
		Type:    "dtmi:org:w3id:rec:Building;1",
		Id:      id,
	}
}

func NewSensor(id string) Entity {
	return Entity{
		Context: "https://dev.realestatecore.io/contexts/Sensor.jsonld",
		Type:    "dtmi:org:brickschema:schema:Brick:Sensor;1",
		Id:      id,
	}
}

func NewObservationEvent(id string) Entity {
	return Entity{
		Context: "https://dev.realestatecore.io/contexts/ObservationEvent.jsonld",
		Type:    "dtmi:org:w3id:rec:ObservationEvent;1",
		Id:      id,
	}
}
