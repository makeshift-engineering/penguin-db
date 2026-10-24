DROP DATABASE IF EXISTS test_airlines;

CREATE DATABASE test_airlines;

\c test_airlines;

DROP TABLE IF EXISTS tickets;

DROP TABLE IF EXISTS passengers;

DROP TABLE IF EXISTS flights;

DROP TABLE IF EXISTS airports;

CREATE TABLE airports (
    id INT PRIMARY KEY,
    code VARCHAR(5),
    name VARCHAR(50),
    city VARCHAR(30)
);

CREATE TABLE flights (
    id INT PRIMARY KEY,
    flight_num VARCHAR(10),
    origin_id INT,
    dest_id INT,
    flight_date VARCHAR(20),
    status VARCHAR(20)
);

CREATE TABLE passengers (
    id INT PRIMARY KEY,
    name VARCHAR(50),
    passport VARCHAR(20),
    nationality VARCHAR(30)
);

CREATE TABLE tickets (
    id INT PRIMARY KEY,
    flight_id INT,
    passenger_id INT,
    seat_num VARCHAR(10),
    price FLOAT
);

INSERT INTO airports VALUES (1, 'JFK', 'John F. Kennedy Intl', 'New York');

INSERT INTO airports VALUES (2, 'LAX', 'Los Angeles Intl', 'Los Angeles');

INSERT INTO airports VALUES (3, 'ORD', 'O Hare Intl', 'Chicago');

INSERT INTO airports VALUES (4, 'LHR', 'Heathrow Airport', 'London');

INSERT INTO passengers VALUES (101, 'Alice Smith', 'US123456', 'USA');

INSERT INTO passengers VALUES (102, 'Bob Jones', 'CA987654', 'Canada');

INSERT INTO passengers VALUES (103, 'Charlie Brown', 'UK555444', 'UK');

INSERT INTO passengers VALUES (104, 'Diana Prince', 'US777888', 'USA');

INSERT INTO passengers VALUES (105, 'Evan Wright', 'AU333222', 'Australia');

INSERT INTO passengers VALUES (106, 'Fiona Gallagher', 'US111222', 'USA');

INSERT INTO passengers VALUES (107, 'George Clark', 'CA444555', 'Canada');

INSERT INTO passengers VALUES (108, 'Hannah Abbott', 'UK888999', 'UK');

INSERT INTO passengers VALUES (109, 'Ian Malcolm', 'US666555', 'USA');

INSERT INTO passengers VALUES (110, 'Julia Roberts', 'AU123789', 'Australia');

INSERT INTO passengers VALUES (111, 'Kevin Bacon', 'US456123', 'USA');

INSERT INTO passengers VALUES (112, 'Laura Croft', 'UK789456', 'UK');

INSERT INTO passengers VALUES (113, 'Michael Scott', 'US321654', 'USA');

INSERT INTO passengers VALUES (114, 'Nina Williams', 'JP987123', 'Japan');

INSERT INTO passengers VALUES (115, 'Oscar Martinez', 'US654987', 'USA');

INSERT INTO passengers VALUES (116, 'Pam Beesly', 'US147258', 'USA');

INSERT INTO passengers VALUES (117, 'Quentin Tarantino', 'US258369', 'USA');

INSERT INTO passengers VALUES (118, 'Rachel Green', 'US369147', 'USA');

INSERT INTO passengers VALUES (119, 'Steve Rogers', 'US951753', 'USA');

INSERT INTO passengers VALUES (120, 'Tony Stark', 'US753951', 'USA');

INSERT INTO flights VALUES (501, 'AA101', 1, 2, '2026-03-01', 'ON_TIME');

INSERT INTO flights VALUES (502, 'UA202', 1, 3, '2026-03-01', 'DELAYED');

INSERT INTO flights VALUES (503, 'BA303', 1, 4, '2026-03-02', 'ON_TIME');

INSERT INTO flights VALUES (504, 'DL404', 2, 1, '2026-03-02', 'ON_TIME');

INSERT INTO flights VALUES (505, 'AA505', 2, 3, '2026-03-03', 'BOARDING');

INSERT INTO flights VALUES (506, 'UA606', 3, 1, '2026-03-03', 'ON_TIME');

INSERT INTO flights VALUES (507, 'BA707', 4, 1, '2026-03-04', 'ON_TIME');

INSERT INTO flights VALUES (508, 'DL808', 3, 2, '2026-03-04', 'CANCELLED');

INSERT INTO flights VALUES (509, 'AA909', 2, 4, '2026-03-05', 'ON_TIME');

INSERT INTO flights VALUES (510, 'UA010', 4, 2, '2026-03-05', 'ON_TIME');

INSERT INTO flights VALUES (511, 'BA111', 1, 2, '2026-03-06', 'ON_TIME');

INSERT INTO flights VALUES (512, 'DL212', 2, 3, '2026-03-06', 'ON_TIME');

INSERT INTO flights VALUES (513, 'AA313', 3, 4, '2026-03-07', 'BOARDING');

INSERT INTO flights VALUES (514, 'UA414', 4, 3, '2026-03-07', 'ON_TIME');

INSERT INTO flights VALUES (515, 'BA515', 1, 3, '2026-03-08', 'ON_TIME');

INSERT INTO flights VALUES (516, 'DL616', 2, 4, '2026-03-08', 'DELAYED');

INSERT INTO flights VALUES (517, 'AA717', 3, 1, '2026-03-09', 'ON_TIME');

INSERT INTO flights VALUES (518, 'UA818', 4, 1, '2026-03-09', 'ON_TIME');

INSERT INTO flights VALUES (519, 'BA919', 1, 4, '2026-03-10', 'ON_TIME');

INSERT INTO flights VALUES (520, 'DL020', 2, 1, '2026-03-10', 'ON_TIME');

INSERT INTO tickets VALUES (901, 501, 101, '12A', 350.00);

INSERT INTO tickets VALUES (902, 501, 102, '12B', 350.00);

INSERT INTO tickets VALUES (903, 502, 103, '14C', 220.00);

INSERT INTO tickets VALUES (904, 503, 104, '02A', 850.00);

INSERT INTO tickets VALUES (905, 504, 105, '15D', 340.00);

INSERT INTO tickets VALUES (906, 505, 106, '18A', 190.00);

INSERT INTO tickets VALUES (907, 506, 107, '20F', 210.00);

INSERT INTO tickets VALUES (908, 507, 108, '04B', 920.00);

INSERT INTO tickets VALUES (909, 509, 109, '08C', 780.00);

INSERT INTO tickets VALUES (910, 510, 110, '11A', 810.00);

INSERT INTO tickets VALUES (911, 511, 111, '16D', 360.00);

INSERT INTO tickets VALUES (912, 512, 112, '17B', 195.00);

INSERT INTO tickets VALUES (913, 513, 113, '22A', 650.00);

INSERT INTO tickets VALUES (914, 514, 114, '05C', 720.00);

INSERT INTO tickets VALUES (915, 515, 115, '19E', 230.00);

INSERT INTO tickets VALUES (916, 516, 116, '10A', 790.00);

INSERT INTO tickets VALUES (917, 517, 117, '21C', 205.00);

INSERT INTO tickets VALUES (918, 518, 118, '03A', 880.00);

INSERT INTO tickets VALUES (919, 519, 119, '01A', 1200.00);

INSERT INTO tickets VALUES (920, 520, 120, '01B', 1200.00);

UPDATE flights SET status = 'ON_TIME' WHERE id = 502;

UPDATE tickets SET price = 370.00 WHERE id = 901;

UPDATE passengers SET nationality = 'USA' WHERE id = 102;

DELETE FROM flights WHERE id = 508;

DELETE FROM passengers WHERE id = 113;

SELECT id, code, name, city FROM airports ORDER BY id ASC;

SELECT id, flight_num, flight_date, status FROM flights WHERE status = 'ON_TIME' ORDER BY id ASC;

SELECT id, name, passport, nationality FROM passengers WHERE nationality = 'USA' ORDER BY id ASC;

SELECT id, flight_id, passenger_id, price FROM tickets WHERE price >= 500.00 ORDER BY price DESC;

SELECT count(*) FROM passengers;

SELECT count(*) FROM flights WHERE origin_id = 1;

SELECT min(price), max(price), avg(price) FROM tickets;

SELECT sum(price) FROM tickets;

SELECT status, count(*) FROM flights GROUP BY status ORDER BY status ASC;

SELECT nationality, count(*) FROM passengers GROUP BY nationality ORDER BY nationality ASC;

SELECT flights.id, flights.flight_num, airports.code FROM flights INNER JOIN airports ON flights.origin_id = airports.id ORDER BY flights.id ASC;

SELECT tickets.id, passengers.name, flights.flight_num, tickets.price FROM tickets INNER JOIN passengers ON tickets.passenger_id = passengers.id INNER JOIN flights ON tickets.flight_id = flights.id ORDER BY tickets.id ASC;

SELECT id, flight_num FROM flights WHERE origin_id = 1 AND status = 'ON_TIME' ORDER BY id ASC;

SELECT id, flight_num FROM flights WHERE origin_id = 2 OR dest_id = 2 ORDER BY id ASC;

SELECT id, flight_num, flight_date FROM flights ORDER BY flight_date ASC LIMIT 5;

SELECT id, flight_num, flight_date FROM flights ORDER BY flight_date ASC LIMIT 5 OFFSET 5;
