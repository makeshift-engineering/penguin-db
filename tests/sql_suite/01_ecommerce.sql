DROP DATABASE IF EXISTS test_ecommerce;

CREATE DATABASE test_ecommerce;

\c test_ecommerce;

DROP TABLE IF EXISTS order_items;

DROP TABLE IF EXISTS orders;

DROP TABLE IF EXISTS products;

DROP TABLE IF EXISTS users;

CREATE TABLE users (
    id INT PRIMARY KEY,
    name VARCHAR(50),
    email VARCHAR(50),
    age INT,
    country VARCHAR(30)
);

CREATE TABLE products (
    id INT PRIMARY KEY,
    title VARCHAR(50),
    category VARCHAR(30),
    price FLOAT,
    stock INT
);

CREATE TABLE orders (
    id INT PRIMARY KEY,
    user_id INT,
    order_date VARCHAR(20),
    total_amount FLOAT,
    status VARCHAR(20)
);

CREATE TABLE order_items (
    id INT PRIMARY KEY,
    order_id INT,
    product_id INT,
    quantity INT,
    unit_price FLOAT
);

INSERT INTO users VALUES (1, 'Alice Smith', 'alice@test.com', 28, 'USA');

INSERT INTO users VALUES (2, 'Bob Jones', 'bob@test.com', 34, 'Canada');

INSERT INTO users VALUES (3, 'Charlie Brown', 'charlie@test.com', 22, 'UK');

INSERT INTO users VALUES (4, 'Diana Prince', 'diana@test.com', 31, 'USA');

INSERT INTO users VALUES (5, 'Evan Wright', 'evan@test.com', 45, 'Australia');

INSERT INTO users VALUES (6, 'Fiona Gallagher', 'fiona@test.com', 26, 'USA');

INSERT INTO users VALUES (7, 'George Clark', 'george@test.com', 39, 'Canada');

INSERT INTO users VALUES (8, 'Hannah Abbott', 'hannah@test.com', 29, 'UK');

INSERT INTO users VALUES (9, 'Ian Malcolm', 'ian@test.com', 52, 'USA');

INSERT INTO users VALUES (10, 'Julia Roberts', 'julia@test.com', 27, 'Australia');

INSERT INTO users VALUES (11, 'Kevin Bacon', 'kevin@test.com', 41, 'USA');

INSERT INTO users VALUES (12, 'Laura Croft', 'laura@test.com', 33, 'UK');

INSERT INTO users VALUES (13, 'Michael Scott', 'michael@test.com', 48, 'USA');

INSERT INTO users VALUES (14, 'Nina Williams', 'nina@test.com', 25, 'Japan');

INSERT INTO users VALUES (15, 'Oscar Martinez', 'oscar@test.com', 36, 'USA');

INSERT INTO users VALUES (16, 'Pam Beesly', 'pam@test.com', 30, 'USA');

INSERT INTO users VALUES (17, 'Quentin Tarantino', 'quentin@test.com', 55, 'USA');

INSERT INTO users VALUES (18, 'Rachel Green', 'rachel@test.com', 29, 'USA');

INSERT INTO users VALUES (19, 'Steve Rogers', 'steve@test.com', 38, 'USA');

INSERT INTO users VALUES (20, 'Tony Stark', 'tony@test.com', 42, 'USA');

INSERT INTO products VALUES (101, 'Mechanical Keyboard', 'Electronics', 120.00, 50);

INSERT INTO products VALUES (102, 'Ergonomic Mouse', 'Electronics', 45.50, 100);

INSERT INTO products VALUES (103, '4K Monitor', 'Electronics', 350.00, 20);

INSERT INTO products VALUES (104, 'Desk Chair', 'Furniture', 220.00, 15);

INSERT INTO products VALUES (105, 'Standing Desk', 'Furniture', 450.00, 10);

INSERT INTO products VALUES (106, 'USB-C Cable', 'Electronics', 12.99, 200);

INSERT INTO products VALUES (107, 'Wireless Headset', 'Electronics', 89.99, 40);

INSERT INTO products VALUES (108, 'Notebook Journal', 'Stationery', 8.50, 150);

INSERT INTO products VALUES (109, 'Gel Pens Set', 'Stationery', 14.25, 80);

INSERT INTO products VALUES (110, 'LED Desk Lamp', 'Furniture', 32.00, 60);

INSERT INTO products VALUES (111, 'Water Bottle', 'Accessories', 18.00, 120);

INSERT INTO products VALUES (112, 'Backpack', 'Accessories', 65.00, 35);

INSERT INTO products VALUES (113, 'Webcam 1080p', 'Electronics', 59.99, 45);

INSERT INTO products VALUES (114, 'Mouse Pad XL', 'Electronics', 19.99, 90);

INSERT INTO products VALUES (115, 'Coffee Mug', 'Accessories', 11.50, 110);

INSERT INTO products VALUES (116, 'Bluetooth Speaker', 'Electronics', 49.99, 50);

INSERT INTO products VALUES (117, 'External Hard Drive', 'Electronics', 85.00, 25);

INSERT INTO products VALUES (118, 'Filing Cabinet', 'Furniture', 135.00, 8);

INSERT INTO products VALUES (119, 'Desk Mat Leather', 'Furniture', 29.99, 70);

INSERT INTO products VALUES (120, 'Desk Organizer', 'Stationery', 16.50, 85);

INSERT INTO orders VALUES (501, 1, '2026-01-10', 165.50, 'COMPLETED');

INSERT INTO orders VALUES (502, 2, '2026-01-11', 450.00, 'COMPLETED');

INSERT INTO orders VALUES (503, 3, '2026-01-12', 350.00, 'PENDING');

INSERT INTO orders VALUES (504, 4, '2026-01-15', 89.99, 'COMPLETED');

INSERT INTO orders VALUES (505, 5, '2026-01-18', 220.00, 'CANCELLED');

INSERT INTO orders VALUES (506, 6, '2026-01-20', 12.99, 'COMPLETED');

INSERT INTO orders VALUES (507, 7, '2026-01-22', 65.00, 'PENDING');

INSERT INTO orders VALUES (508, 8, '2026-01-25', 120.00, 'COMPLETED');

INSERT INTO orders VALUES (509, 9, '2026-01-28', 59.99, 'COMPLETED');

INSERT INTO orders VALUES (510, 10, '2026-02-01', 49.99, 'COMPLETED');

INSERT INTO orders VALUES (511, 11, '2026-02-03', 45.50, 'COMPLETED');

INSERT INTO orders VALUES (512, 12, '2026-02-05', 85.00, 'PENDING');

INSERT INTO orders VALUES (513, 13, '2026-02-08', 32.00, 'COMPLETED');

INSERT INTO orders VALUES (514, 14, '2026-02-10', 18.00, 'COMPLETED');

INSERT INTO orders VALUES (515, 15, '2026-02-12', 220.00, 'COMPLETED');

INSERT INTO orders VALUES (516, 16, '2026-02-15', 14.25, 'COMPLETED');

INSERT INTO orders VALUES (517, 17, '2026-02-18', 135.00, 'CANCELLED');

INSERT INTO orders VALUES (518, 18, '2026-02-20', 29.99, 'COMPLETED');

INSERT INTO orders VALUES (519, 19, '2026-02-22', 16.50, 'COMPLETED');

INSERT INTO orders VALUES (520, 20, '2026-02-25', 120.00, 'COMPLETED');

INSERT INTO order_items VALUES (1001, 501, 101, 1, 120.00);

INSERT INTO order_items VALUES (1002, 501, 102, 1, 45.50);

INSERT INTO order_items VALUES (1003, 502, 105, 1, 450.00);

INSERT INTO order_items VALUES (1004, 503, 103, 1, 350.00);

INSERT INTO order_items VALUES (1005, 504, 107, 1, 89.99);

INSERT INTO order_items VALUES (1006, 505, 104, 1, 220.00);

INSERT INTO order_items VALUES (1007, 506, 106, 1, 12.99);

INSERT INTO order_items VALUES (1008, 507, 112, 1, 65.00);

INSERT INTO order_items VALUES (1009, 508, 101, 1, 120.00);

INSERT INTO order_items VALUES (1010, 509, 113, 1, 59.99);

INSERT INTO order_items VALUES (1011, 510, 116, 1, 49.99);

INSERT INTO order_items VALUES (1012, 511, 102, 1, 45.50);

INSERT INTO order_items VALUES (1013, 512, 117, 1, 85.00);

INSERT INTO order_items VALUES (1014, 513, 110, 1, 32.00);

INSERT INTO order_items VALUES (1015, 514, 111, 1, 18.00);

INSERT INTO order_items VALUES (1016, 515, 104, 1, 220.00);

INSERT INTO order_items VALUES (1017, 516, 109, 1, 14.25);

INSERT INTO order_items VALUES (1018, 517, 118, 1, 135.00);

INSERT INTO order_items VALUES (1019, 518, 119, 1, 29.99);

INSERT INTO order_items VALUES (1020, 519, 120, 1, 16.50);

UPDATE users SET age = 29 WHERE id = 1;

UPDATE products SET stock = 48 WHERE id = 101;

UPDATE orders SET status = 'COMPLETED' WHERE id = 503;

DELETE FROM users WHERE id = 17;

DELETE FROM products WHERE id = 118;

SELECT id, name, email FROM users ORDER BY id ASC;

SELECT id, name, age, country FROM users WHERE age >= 30 ORDER BY age DESC;

SELECT id, title, price FROM products WHERE category = 'Electronics' ORDER BY price DESC;

SELECT id, title, stock FROM products WHERE stock < 30 ORDER BY id ASC;

SELECT id, user_id, total_amount, status FROM orders WHERE status = 'COMPLETED' ORDER BY total_amount DESC;

SELECT id, user_id, order_date FROM orders WHERE status = 'PENDING' ORDER BY id ASC;

SELECT count(*) FROM users;

SELECT count(*) FROM products WHERE category = 'Electronics';

SELECT min(price), max(price), avg(price) FROM products;

SELECT sum(total_amount) FROM orders WHERE status = 'COMPLETED';

SELECT category, count(*) FROM products GROUP BY category ORDER BY category ASC;

SELECT country, count(*) FROM users GROUP BY country ORDER BY country ASC;

SELECT status, count(*) FROM orders GROUP BY status ORDER BY status ASC;

SELECT id, name FROM users WHERE country = 'USA' AND age < 35 ORDER BY id ASC;

SELECT id, title, price FROM products WHERE category = 'Furniture' OR price > 100.00 ORDER BY id ASC;

SELECT id, name FROM users WHERE NOT (country = 'USA') ORDER BY id ASC;

SELECT id, title, price FROM products ORDER BY price DESC LIMIT 5;

SELECT id, name, age FROM users ORDER BY age ASC LIMIT 5 OFFSET 5;

SELECT users.id, users.name, orders.id, orders.total_amount FROM users INNER JOIN orders ON users.id = orders.user_id ORDER BY users.id ASC;

SELECT orders.id, orders.order_date, order_items.product_id, order_items.quantity FROM orders INNER JOIN order_items ON orders.id = order_items.order_id ORDER BY orders.id ASC;

SELECT products.id, products.title, order_items.order_id, order_items.quantity FROM products INNER JOIN order_items ON products.id = order_items.product_id ORDER BY products.id ASC;

SELECT count(*) FROM order_items WHERE unit_price > 50.00;
