DROP DATABASE IF EXISTS test_logistics;

CREATE DATABASE test_logistics;

\c test_logistics;

DROP TABLE IF EXISTS shipments;

DROP TABLE IF EXISTS inventory;

DROP TABLE IF EXISTS suppliers;

DROP TABLE IF EXISTS warehouses;

CREATE TABLE warehouses (
    id INT PRIMARY KEY,
    location VARCHAR(50),
    capacity INT
);

CREATE TABLE suppliers (
    id INT PRIMARY KEY,
    name VARCHAR(50),
    country VARCHAR(30)
);

CREATE TABLE inventory (
    id INT PRIMARY KEY,
    warehouse_id INT,
    supplier_id INT,
    item_name VARCHAR(50),
    quantity INT,
    unit_cost FLOAT
);

CREATE TABLE shipments (
    id INT PRIMARY KEY,
    inventory_id INT,
    origin VARCHAR(50),
    destination VARCHAR(50),
    status VARCHAR(20)
);

INSERT INTO warehouses VALUES (1, 'Chicago Hub', 10000);

INSERT INTO warehouses VALUES (2, 'Dallas Logistics Center', 15000);

INSERT INTO warehouses VALUES (3, 'Los Angeles Gateway', 20000);

INSERT INTO warehouses VALUES (4, 'Atlanta Depot', 12000);

INSERT INTO suppliers VALUES (101, 'Global Tech Components', 'Taiwan');

INSERT INTO suppliers VALUES (102, 'Apex Freight Supply', 'USA');

INSERT INTO suppliers VALUES (103, 'Sino Logistics Group', 'China');

INSERT INTO suppliers VALUES (104, 'Euro Parts GmbH', 'Germany');

INSERT INTO suppliers VALUES (105, 'Nippon Electronics', 'Japan');

INSERT INTO inventory VALUES (501, 1, 101, 'Semiconductor Chips', 500, 25.50);

INSERT INTO inventory VALUES (502, 1, 102, 'Cardboard Boxes XL', 2000, 1.20);

INSERT INTO inventory VALUES (503, 1, 105, 'Lithium Batteries', 800, 15.00);

INSERT INTO inventory VALUES (504, 2, 102, 'Pallet Wraps', 300, 12.00);

INSERT INTO inventory VALUES (505, 2, 103, 'Plastic Containers', 1500, 4.50);

INSERT INTO inventory VALUES (506, 2, 104, 'Industrial Valves', 150, 120.00);

INSERT INTO inventory VALUES (507, 3, 101, 'Display Panels 15in', 400, 85.00);

INSERT INTO inventory VALUES (508, 3, 103, 'Solar Inverters', 100, 350.00);

INSERT INTO inventory VALUES (509, 3, 105, 'Camera Sensors', 600, 45.00);

INSERT INTO inventory VALUES (510, 4, 102, 'Packing Tape Rolls', 5000, 0.80);

INSERT INTO inventory VALUES (511, 4, 104, 'Hydraulic Pumps', 80, 210.00);

INSERT INTO inventory VALUES (512, 1, 101, 'Memory Modules 16GB', 750, 42.00);

INSERT INTO inventory VALUES (513, 2, 103, 'LED Drivers', 1200, 8.50);

INSERT INTO inventory VALUES (514, 3, 104, 'Electric Motors', 90, 180.00);

INSERT INTO inventory VALUES (515, 4, 105, 'Fiber Optic Cables', 3000, 3.20);

INSERT INTO inventory VALUES (516, 1, 102, 'Bubble Wrap Rolls', 450, 18.50);

INSERT INTO inventory VALUES (517, 2, 101, 'Power Supply Units', 350, 55.00);

INSERT INTO inventory VALUES (518, 3, 103, 'Aluminum Sheets', 250, 65.00);

INSERT INTO inventory VALUES (519, 4, 104, 'Steel Fasteners Box', 4000, 2.50);

INSERT INTO inventory VALUES (520, 1, 105, 'Thermal Paste Tubes', 2500, 1.80);

INSERT INTO shipments VALUES (901, 501, 'Chicago Hub', 'New York', 'DELIVERED');

INSERT INTO shipments VALUES (902, 503, 'Chicago Hub', 'Seattle', 'IN_TRANSIT');

INSERT INTO shipments VALUES (903, 505, 'Dallas Logistics Center', 'Miami', 'DELIVERED');

INSERT INTO shipments VALUES (904, 506, 'Dallas Logistics Center', 'Houston', 'PENDING');

INSERT INTO shipments VALUES (905, 507, 'Los Angeles Gateway', 'San Francisco', 'DELIVERED');

INSERT INTO shipments VALUES (906, 508, 'Los Angeles Gateway', 'Phoenix', 'IN_TRANSIT');

INSERT INTO shipments VALUES (907, 510, 'Atlanta Depot', 'Charlotte', 'DELIVERED');

INSERT INTO shipments VALUES (908, 511, 'Atlanta Depot', 'Nashville', 'PENDING');

INSERT INTO shipments VALUES (909, 512, 'Chicago Hub', 'Boston', 'DELIVERED');

INSERT INTO shipments VALUES (910, 513, 'Dallas Logistics Center', 'Denver', 'IN_TRANSIT');

INSERT INTO shipments VALUES (911, 514, 'Los Angeles Gateway', 'Las Vegas', 'DELIVERED');

INSERT INTO shipments VALUES (912, 515, 'Atlanta Depot', 'Orlando', 'DELIVERED');

INSERT INTO shipments VALUES (913, 516, 'Chicago Hub', 'Detroit', 'PENDING');

INSERT INTO shipments VALUES (914, 517, 'Dallas Logistics Center', 'Austin', 'DELIVERED');

INSERT INTO shipments VALUES (915, 518, 'Los Angeles Gateway', 'Salt Lake City', 'IN_TRANSIT');

INSERT INTO shipments VALUES (916, 519, 'Atlanta Depot', 'Tampa', 'DELIVERED');

INSERT INTO shipments VALUES (917, 520, 'Chicago Hub', 'Philadelphia', 'DELIVERED');

INSERT INTO shipments VALUES (918, 502, 'Chicago Hub', 'Columbus', 'DELIVERED');

INSERT INTO shipments VALUES (919, 504, 'Dallas Logistics Center', 'San Antonio', 'IN_TRANSIT');

INSERT INTO shipments VALUES (920, 509, 'Los Angeles Gateway', 'San Diego', 'DELIVERED');

UPDATE inventory SET quantity = 550 WHERE id = 501;

UPDATE warehouses SET capacity = 11000 WHERE id = 1;

UPDATE shipments SET status = 'DELIVERED' WHERE id = 902;

DELETE FROM inventory WHERE id = 519;

DELETE FROM shipments WHERE id = 916;
