DROP DATABASE IF EXISTS test_iot_sensors;

CREATE DATABASE test_iot_sensors;

\c test_iot_sensors;

DROP TABLE IF EXISTS alerts;

DROP TABLE IF EXISTS readings;

DROP TABLE IF EXISTS devices;

DROP TABLE IF EXISTS locations;

CREATE TABLE locations (
    id INT PRIMARY KEY,
    building VARCHAR(50),
    floor INT
);

CREATE TABLE devices (
    id INT PRIMARY KEY,
    location_id INT,
    device_type VARCHAR(30),
    model VARCHAR(30),
    active BOOL
);

CREATE TABLE readings (
    id INT PRIMARY KEY,
    device_id INT,
    sensor_type VARCHAR(30),
    reading_val FLOAT,
    recorded_at VARCHAR(20)
);

CREATE TABLE alerts (
    id INT PRIMARY KEY,
    device_id INT,
    severity VARCHAR(20),
    message VARCHAR(50)
);

INSERT INTO locations VALUES (1, 'Headquarters Tower', 1);

INSERT INTO locations VALUES (2, 'Headquarters Tower', 2);

INSERT INTO locations VALUES (3, 'Headquarters Tower', 3);

INSERT INTO locations VALUES (4, 'Research Lab', 1);

INSERT INTO locations VALUES (5, 'Research Lab', 2);

INSERT INTO devices VALUES (101, 1, 'Temperature Sensor', 'TEMP-v1', true);

INSERT INTO devices VALUES (102, 1, 'Humidity Sensor', 'HUM-v2', true);

INSERT INTO devices VALUES (103, 1, 'Motion Detector', 'MOT-v1', true);

INSERT INTO devices VALUES (104, 2, 'Temperature Sensor', 'TEMP-v1', true);

INSERT INTO devices VALUES (105, 2, 'CO2 Sensor', 'AIR-v3', true);

INSERT INTO devices VALUES (106, 2, 'Light Sensor', 'LUX-v1', false);

INSERT INTO devices VALUES (107, 3, 'Temperature Sensor', 'TEMP-v2', true);

INSERT INTO devices VALUES (108, 3, 'Pressure Gauge', 'PRES-v1', true);

INSERT INTO devices VALUES (109, 3, 'Smoke Detector', 'SMK-v1', true);

INSERT INTO devices VALUES (110, 4, 'Temperature Sensor', 'TEMP-v2', true);

INSERT INTO devices VALUES (111, 4, 'Vibration Monitor', 'VIB-v1', true);

INSERT INTO devices VALUES (112, 4, 'Radiation Counter', 'RAD-v1', true);

INSERT INTO devices VALUES (113, 5, 'Temperature Sensor', 'TEMP-v1', true);

INSERT INTO devices VALUES (114, 5, 'Humidity Sensor', 'HUM-v2', true);

INSERT INTO devices VALUES (115, 5, 'Gas Detector', 'GAS-v1', true);

INSERT INTO devices VALUES (116, 1, 'Airflow Sensor', 'FLOW-v1', true);

INSERT INTO devices VALUES (117, 2, 'Noise Monitor', 'DEC-v1', true);

INSERT INTO devices VALUES (118, 3, 'Voltage Sensor', 'VOLT-v1', true);

INSERT INTO devices VALUES (119, 4, 'Current Meter', 'AMP-v1', true);

INSERT INTO devices VALUES (120, 5, 'Water Leak Sensor', 'LEAK-v1', false);

INSERT INTO readings VALUES (501, 101, 'TEMPERATURE', 21.5, '2026-03-01 08:00');

INSERT INTO readings VALUES (502, 102, 'HUMIDITY', 45.0, '2026-03-01 08:00');

INSERT INTO readings VALUES (503, 104, 'TEMPERATURE', 22.1, '2026-03-01 08:05');

INSERT INTO readings VALUES (504, 105, 'CO2', 410.0, '2026-03-01 08:05');

INSERT INTO readings VALUES (505, 107, 'TEMPERATURE', 23.8, '2026-03-01 08:10');

INSERT INTO readings VALUES (506, 108, 'PRESSURE', 1013.25, '2026-03-01 08:10');

INSERT INTO readings VALUES (507, 110, 'TEMPERATURE', 19.5, '2026-03-01 08:15');

INSERT INTO readings VALUES (508, 111, 'VIBRATION', 0.02, '2026-03-01 08:15');

INSERT INTO readings VALUES (509, 113, 'TEMPERATURE', 24.0, '2026-03-01 08:20');

INSERT INTO readings VALUES (510, 114, 'HUMIDITY', 52.0, '2026-03-01 08:20');

INSERT INTO readings VALUES (511, 101, 'TEMPERATURE', 22.0, '2026-03-01 09:00');

INSERT INTO readings VALUES (512, 102, 'HUMIDITY', 46.5, '2026-03-01 09:00');

INSERT INTO readings VALUES (513, 104, 'TEMPERATURE', 23.0, '2026-03-01 09:05');

INSERT INTO readings VALUES (514, 105, 'CO2', 425.0, '2026-03-01 09:05');

INSERT INTO readings VALUES (515, 107, 'TEMPERATURE', 25.1, '2026-03-01 09:10');

INSERT INTO readings VALUES (516, 108, 'PRESSURE', 1012.80, '2026-03-01 09:10');

INSERT INTO readings VALUES (517, 110, 'TEMPERATURE', 20.1, '2026-03-01 09:15');

INSERT INTO readings VALUES (518, 113, 'TEMPERATURE', 24.8, '2026-03-01 09:20');

INSERT INTO readings VALUES (519, 115, 'GAS', 0.01, '2026-03-01 09:20');

INSERT INTO readings VALUES (520, 118, 'VOLTAGE', 120.2, '2026-03-01 09:25');

INSERT INTO alerts VALUES (901, 105, 'HIGH', 'Elevated CO2 level detected');

INSERT INTO alerts VALUES (902, 107, 'MEDIUM', 'Temperature exceeding baseline');

INSERT INTO alerts VALUES (903, 109, 'LOW', 'Battery self-test reminder');

INSERT INTO alerts VALUES (904, 112, 'CRITICAL', 'Radiation threshold exceeded');

INSERT INTO alerts VALUES (905, 115, 'HIGH', 'Trace gas detected in lab');

INSERT INTO alerts VALUES (906, 117, 'LOW', 'Ambient noise spike');

INSERT INTO alerts VALUES (907, 120, 'HIGH', 'Water sensor disconnected');

INSERT INTO alerts VALUES (908, 101, 'LOW', 'Firmware update available');

INSERT INTO alerts VALUES (909, 104, 'MEDIUM', 'HVAC calibration required');

INSERT INTO alerts VALUES (910, 108, 'LOW', 'Pressure sensor steady');

UPDATE devices SET active = true WHERE id = 106;

UPDATE devices SET active = true WHERE id = 120;

UPDATE alerts SET severity = 'RESOLVED' WHERE id = 901;

DELETE FROM alerts WHERE id = 908;

DELETE FROM devices WHERE id = 119;
