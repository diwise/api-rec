# API-REC

API:et är inspirerat/baserat på [specifikationen](https://github.com/RealEstateCore/rec/blob/main/API/REST/RealEstateCore_REST_specification.md) för REST-API:et i [RealEstateCore](https://dev.realestatecore.io/) (även kallat REC).

**POST** `/api/spaces`

**GET** `/api/spaces`

**POST** `/api/buildings`

**GET** `/api/buildings`

**POST** `/api/sensors`

**GET** `/api/sensors`

**POST** `/api/observations`

**GET** `/api/observations`

`root[id]` - id för root-objekt

`root[type]` - typ för root-objekt

`hasObservationTime[starting]` - starttidpunkt för tidsspann med observationer

`hasObservationTime[ending]` - sluttidpunkt för tidsspann med observationer

`page` aktuell *sida* att hämta

`size` antal objekt (buildings, sensors, observations o.dyl.) i varje *sida*

## Sensorvärden

`api-rec` tar emot meddelanden och lagrar data i ett tidsserie format.

### Cloudevents

`iot-events` konfigureras att POST ett cloudevent till api-rec endpoint `/api/cloudevents`. Event som ska POST:as är `message.accepted` och `function.updated`.

```yaml
subscribers:
  - id: api-rec-devices
    name: MessageAccepted
    type: message.accepted
    endpoint: http://api-rec:8080/api/cloudevents
    source: github.com/diwise/iot-agent
    eventType: message.accepted
    tenants:
      - default
    entities:
  - id: api-rec-functions
    name: FunctionUpdated
    type: function.updated
    endpoint: http://api-rec:8080/api/cloudevents
    source: github.com/diwise/iot-core
    eventType: function.updated
    tenants:
      - default
    entities:
```

Flödet blir då: sensor -> iot-agent -> iot-events -> cloudevents -> api-rec

`api-rec` kommer tolka händelserna och skapa `observations` från dem.

### REST

-> api-rec

En `observation` kan också POST:as direkt till `/api/observations`.

```json
{
  "format": "rec3.3",
  "deviceId": "https://recref.com/device/64b65a99-a53c-47f5-b959-1c7a641d82d8",
  "observations": [
    {
      "observationTime": "2019-05-27T20:07:44Z",
      "value": 16.1,
      "quantityKind": "https://w3id.org/rec/core/Temperature",
      "sensorId" : "https://recref.com/sensor/e0d5120b-90f1-48d6-a47f-f8ccd7727b04"
    }
  ]
}
```

### QuantityKind

[Units](https://doc.realestatecore.io/3.3/units.html)

Ett urval av typer som finns i spec för REC:

- Concentration
- Energy
- Force
- Illuminance
- Power
- Pressure
- RelativeHumidity
- Resistance
- Temperature
- Volume

Några extra har skapats

- diwise:AirQuality
- diwise:DigitalInput
- diwise:Level
- diwise:Lifebuoy
- diwise:Presence
- diwise:Timer

och fler eller andra kommer skapas vid behov.

[https://github.com/RealEstateCore/rec/blob/main/API/Edge/edge_message.schema.json](https://github.com/RealEstateCore/rec/blob/main/API/Edge/edge_message.schema.json)

Skillnden mellan `deviceId` och `sensorId` är att ett `device` kan ha en eller flera `sensor`er i samma "låda".

## Skapa struktur

API för att stukturera fastigheter, byggnader, våningar, rum, m.m. Vi kan behöva fler/andra modeller från REC.

För närvarande finns endpoints för `spaces`, `buildings` och `sensors`.

**POST** `/spaces`

```json
{
  "@context": "https://dev.realestatecore.io/contexts/Space.jsonld",
  "@id": "00f67d60-d4d4-4bd5-af32-cf6c9b9310ec",
  "@type": "dtmi:org:w3id:rec:Space;1"
}
```

**POST** `/buildings`

```json
{
  "@context": "https://dev.realestatecore.io/contexts/Building.jsonld",
  "@id": "79b30db6-c5d3-4cd1-a438-6d8954b330ad",
  "@type": "dtmi:org:w3id:rec:Building;1",
  "isPartOf" : {
        "@id": "00f67d60-d4d4-4bd5-af32-cf6c9b9310ec",
        "@type": "dtmi:org:w3id:rec:Space;1"
  }
}
```

**POST** `/sensors`

```json
{
  "@context": "https://dev.realestatecore.io/contexts/Sensor.jsonld",
  "@id": "76bb4d31-1167-49e0-8766-768eb47c47e2",
  "@type": "dtmi:org:brickschema:schema:Brick:Sensor;1",
  "isPartOf" : {
        "@id": "79b30db6-c5d3-4cd1-a438-6d8954b330ad",
        "@type": "dtmi:org:w3id:rec:Building;1"
  }
}
```

`isPartOf` skapar relation mellan entiteter. Alla modeller har fler properties för metadata som inte finns med i *spiken*.

## Hämta data

Exempel med `/sensors`.

`page=0` och `size=10` är default om inget annat anges. Med dessa parametrar kan man hämta delar av dataset:et. `hydra:totalItems` kommer att ha totalt antal objekt i det fullständiga svaret.

`root[type]` och `root[id]` finns inte i spec, men faller in under [Advanced queries](https://github.com/RealEstateCore/rec/blob/main/API/REST/RealEstateCore_REST_specification.md#advanced-queries) och är tänkt svara på frågor som "ge mig alla sensorer i byggnad X".

`hydra:view` visas enbart om det finns en uppdelning av dataset:et.

**Obs** Används `root[type]` och `root[id]` så kommer inte `page=0` och/eller `size=10` att påverka något, utan då hämtas hela resultatet i samma fråga.

```json
{
  "@context": "http://www.w3.org/ns/hydra/context.jsonld",
  "@id": "/api/sensors",
  "@type": "hydra:Collection",
  "hydra:totalItems": 32,
  "hydra:member": [
    {
      "@context": "https://dev.realestatecore.io/contexts/Sensor.jsonld",
      "@id": "76bb4d31-1167-49e0-8766-768eb47c47e2",
      "@type": "dtmi:org:brickschema:schema:Brick:Sensor;1",
    },
    ...
  ],
  "hydra:view": {
      "@id": "/api/sensors?page=3",
      "@type": "hydra:PartialCollectionView",
      "first": "/api/sensors?page=0&size=10",
      "previous": "/api/sensors?page=2&size=10",
      "last": "/api/sensors?page=3&size=10"
  }  
}
```

### Spaces, Buildings & Sensors

**GET** `/sensors?root[type]=building&root[id]=79b30db6-c5d3-4cd1-a438-6d8954b330ad`

Hämtar alla sensorer som finns i byggnaden med id `79b30db6-c5d3-4cd1-a438-6d8954b330ad`. `type` måste anges då olika typer (spaces, buildings o.dyl.) kan ha samma ID.

```json
{
  "@context": "http://www.w3.org/ns/hydra/context.jsonld",
  "@id": "/api/sensors",
  "@type": "hydra:Collection",
  "hydra:totalItems": 32,
  "hydra:member": [
    {
      "@context": "https://dev.realestatecore.io/contexts/Sensor.jsonld",
      "@id": "76bb4d31-1167-49e0-8766-768eb47c47e2",
      "@type": "dtmi:org:brickschema:schema:Brick:Sensor;1",
    },
    ...
  ]  
}
```

### Observations

Se [Time interval queries](https://github.com/RealEstateCore/rec/blob/main/API/REST/RealEstateCore_REST_specification.md#time-interval-queries) för information.

För `/observations` finns `?hasObservationTime[starting]` och `hasObservationTime[ending]` för att få ut data för ett visst tidsintervall.

`page=0` och `size=10` funkar för observations på samma sätt som för t.ex. `/sensors`.

**GET** `/observations?sensor_id=76bb4d31-1167-49e0-8766-768eb47c47e2&hasObservationTime[starting]=2023-08-01&hasObservationTime[ending]=2023-08-31`

```json
{
    "@context": "http://www.w3.org/ns/hydra/context.jsonld",
    "@id": "/api/observations",
    "@type": "hydra:Collection",
    "hydra:totalItems": 78,
    "hydra:member": [
        {
            "observationTime": "2023-08-24T09:18:29Z",
            "value": 12380400000000,
            "quantityKind": "Energy",
            "sensorId": "vp1-em01"
        },
       ...
    ],
    "hydra:view": {
        "@id": "/observations?sensor_id=76bb4d31-1167-49e0-8766-768eb47c47e2&hasObservationTime[starting]=2023-08-01&hasObservationTime[ending]=2023-08-31",
        "@type": "hydra:PartialCollectionView",
        "first": "/observations?sensor_id=76bb4d31-1167-49e0-8766-768eb47c47e2&hasObservationTime[starting]=2023-08-01&hasObservationTime[ending]=2023-08-31&page=0&size=10",
        "previous": "/observations?sensor_id=76bb4d31-1167-49e0-8766-768eb47c47e2&hasObservationTime[starting]=2023-08-01&hasObservationTime[ending]=2023-08-31&page=2&size=10",
        "last": "/observations?sensor_id=76bb4d31-1167-49e0-8766-768eb47c47e2&hasObservationTime[starting]=2023-08-01&hasObservationTime[ending]=2023-08-31&page=3&size=10"
    }      
}
```

*Det finns logik som hindrar att samma värde lagras flera gånger inom en tidsperiod (nu 1 minut), dvs om sensor-X skickar värdet `42` n gånger inom samma tidsperiod kommer enbart värdet lagras första gången, de andra gångerna kastas värdet. Om sensorn däremot skickar `42`, `43`, `42` inom samma tidsperiod kommer alla tre värden att lagras.*

## Databas

En graf skapas med två tabeller tills det behövs en riktig grafdatabashanterare.

### DDL

```sql
CREATE TABLE IF NOT EXISTS entity (
    node_id        BIGSERIAL,
    entity_id      TEXT NOT NULL,
    entity_type    TEXT NOT NULL,
    entity_context TEXT NOT NULL,
    PRIMARY KEY (node_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS entity_entity_type_entity_id_unique_indx ON entity (entity_type, entity_id);

CREATE TABLE IF NOT EXISTS  relation (
    parent        BIGINT NOT NULL,
    child         BIGINT NOT NULL,
    PRIMARY KEY (parent, child)
);

CREATE INDEX IF NOT EXISTS relation_child_parent_indx ON relation(child, parent);

CREATE TABLE IF NOT EXISTS observations (
    observation_id    BIGSERIAL PRIMARY KEY,
    device_id         TEXT NOT NULL,
    sensor_id         TEXT NOT NULL,
    observation_time  TIMESTAMPTZ NOT NULL,
    value             NUMERIC NULL,
    value_string      TEXT NULL,
    value_boolean     BOOLEAN NULL,
    quantity_kind     TEXT NOT NULL,
    UNIQUE NULLS NOT DISTINCT (device_id, sensor_id, observation_time, value, value_string, value_boolean, quantity_kind)
);

```

**OBS!** `NULL NOT DISTINCT` kräver Postgres 15 eller senare.

### SQL

SQL för att svara på frågor som t.ex. "ge mig alla sensorer i byggnad X"

```sql
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
ORDER BY traverse.entity_id ASC
```

PK för en entitet är `id` + `type`. `$1` och `$2` pekar ut root-entiteten och `$3` den typ som ska hittas bland kopplade entiteter.

## Funderingar

### Hämta data

Möjligen måste `type` vara `dtmi:org:w3id:rec:Building;1` och inte bara `building`. Men det finns en CSV med översättningar för [endpoints](https://github.com/RealEstateCore/rec/blob/main/API/REST/Endpoints.csv).

### Databas

Bättre säkerhet för *magiska strängar* om de läggs in i en ENUM?

```sql
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

CREATE TYPE quantity_kind AS ENUM (
  'diwise:AirQuality'
  'diwise:DigitalInput',
  'diwise:Presence',
  'Acceleration',
  'Angle',
  'AngularAcceleration',
  'AngularVelocity',
  'Area',
  'Capacitance',
  'Concentration',
  'Conductivity',
  'DataRate',
  'DataSize',
  'Density',
  'Distance',
  'Efficiency',
  'ElectricCharge',
  'ElectricCurrent',
  'Energy',
  'Force',
  'Frequency',
  'Illuminance',
  'Inductance',
  'Irradiance',
  'Length',
  'Luminance',
  'LuminousFlux',
  'LuminousIntensity',
  'MagneticFlux',
  'MagneticFluxDensity',
  'Mass',
  'MassFlowRate',
  'Power',
  'PowerFactor',
  'Pressure',
  'RelativeHumidity',
  'Resistance',
  'SoundPressureLevel',
  'Temperature',
  'Thrust',
  'Time',
  'Torque',
  'Velocity',
  'Voltage',
  'Volume',
  'VolumeFlowRate'
);
```

## Kvar att göra

- säkerhet
- bättre översättning mellan lwm2m och REC
- bättre översättning mellan functions och REC
- test
- ???
