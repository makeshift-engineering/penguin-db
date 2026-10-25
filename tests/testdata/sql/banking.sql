DROP DATABASE IF EXISTS test_banking;

CREATE DATABASE test_banking;

\c test_banking;

DROP TABLE IF EXISTS transactions;

DROP TABLE IF EXISTS accounts;

DROP TABLE IF EXISTS customers;

DROP TABLE IF EXISTS branches;

CREATE TABLE branches (
    id INT PRIMARY KEY,
    branch_name VARCHAR(50),
    city VARCHAR(30)
);

CREATE TABLE customers (
    id INT PRIMARY KEY,
    branch_id INT,
    name VARCHAR(50),
    credit_score INT
);

CREATE TABLE accounts (
    id INT PRIMARY KEY,
    customer_id INT,
    account_type VARCHAR(20),
    balance FLOAT
);

CREATE TABLE transactions (
    id INT PRIMARY KEY,
    account_id INT,
    txn_type VARCHAR(20),
    amount FLOAT,
    txn_date VARCHAR(20)
);

INSERT INTO branches VALUES (1, 'Downtown Main', 'New York');

INSERT INTO branches VALUES (2, 'Financial District', 'Chicago');

INSERT INTO branches VALUES (3, 'Silicon Valley', 'San Francisco');

INSERT INTO branches VALUES (4, 'Metro South', 'Houston');

INSERT INTO customers VALUES (101, 1, 'Alice Smith', 750);

INSERT INTO customers VALUES (102, 1, 'Bob Jones', 680);

INSERT INTO customers VALUES (103, 1, 'Charlie Brown', 720);

INSERT INTO customers VALUES (104, 2, 'Diana Prince', 810);

INSERT INTO customers VALUES (105, 2, 'Evan Wright', 640);

INSERT INTO customers VALUES (106, 2, 'Fiona Gallagher', 790);

INSERT INTO customers VALUES (107, 3, 'George Clark', 700);

INSERT INTO customers VALUES (108, 3, 'Hannah Abbott', 830);

INSERT INTO customers VALUES (109, 3, 'Ian Malcolm', 610);

INSERT INTO customers VALUES (110, 4, 'Julia Roberts', 760);

INSERT INTO customers VALUES (111, 4, 'Kevin Bacon', 690);

INSERT INTO customers VALUES (112, 4, 'Laura Croft', 740);

INSERT INTO customers VALUES (113, 1, 'Michael Scott', 580);

INSERT INTO customers VALUES (114, 1, 'Nina Williams', 770);

INSERT INTO customers VALUES (115, 2, 'Oscar Martinez', 820);

INSERT INTO customers VALUES (116, 2, 'Pam Beesly', 710);

INSERT INTO customers VALUES (117, 3, 'Quentin Tarantino', 660);

INSERT INTO customers VALUES (118, 3, 'Rachel Green', 730);

INSERT INTO customers VALUES (119, 4, 'Steve Rogers', 850);

INSERT INTO customers VALUES (120, 4, 'Tony Stark', 840);

INSERT INTO accounts VALUES (501, 101, 'CHECKING', 4500.50);

INSERT INTO accounts VALUES (502, 101, 'SAVINGS', 12000.00);

INSERT INTO accounts VALUES (503, 102, 'CHECKING', 1800.25);

INSERT INTO accounts VALUES (504, 103, 'CHECKING', 9500.00);

INSERT INTO accounts VALUES (505, 104, 'SAVINGS', 45000.00);

INSERT INTO accounts VALUES (506, 105, 'CHECKING', 850.00);

INSERT INTO accounts VALUES (507, 106, 'SAVINGS', 22000.00);

INSERT INTO accounts VALUES (508, 107, 'CHECKING', 3400.75);

INSERT INTO accounts VALUES (509, 108, 'SAVINGS', 68000.00);

INSERT INTO accounts VALUES (510, 109, 'CHECKING', 420.00);

INSERT INTO accounts VALUES (511, 110, 'CHECKING', 5600.00);

INSERT INTO accounts VALUES (512, 111, 'SAVINGS', 15000.00);

INSERT INTO accounts VALUES (513, 112, 'CHECKING', 2900.00);

INSERT INTO accounts VALUES (514, 113, 'CHECKING', 120.50);

INSERT INTO accounts VALUES (515, 114, 'SAVINGS', 18500.00);

INSERT INTO accounts VALUES (516, 115, 'SAVINGS', 54000.00);

INSERT INTO accounts VALUES (517, 116, 'CHECKING', 3100.00);

INSERT INTO accounts VALUES (518, 117, 'CHECKING', 8900.00);

INSERT INTO accounts VALUES (519, 119, 'SAVINGS', 95000.00);

INSERT INTO accounts VALUES (520, 120, 'SAVINGS', 125000.00);

INSERT INTO transactions VALUES (901, 501, 'DEPOSIT', 1000.00, '2026-01-05');

INSERT INTO transactions VALUES (902, 501, 'WITHDRAWAL', 200.00, '2026-01-06');

INSERT INTO transactions VALUES (903, 502, 'DEPOSIT', 5000.00, '2026-01-10');

INSERT INTO transactions VALUES (904, 503, 'WITHDRAWAL', 150.00, '2026-01-12');

INSERT INTO transactions VALUES (905, 504, 'DEPOSIT', 2500.00, '2026-01-15');

INSERT INTO transactions VALUES (906, 505, 'DEPOSIT', 10000.00, '2026-01-18');

INSERT INTO transactions VALUES (907, 506, 'WITHDRAWAL', 100.00, '2026-01-20');

INSERT INTO transactions VALUES (908, 507, 'DEPOSIT', 3000.00, '2026-01-22');

INSERT INTO transactions VALUES (909, 508, 'WITHDRAWAL', 400.00, '2026-01-25');

INSERT INTO transactions VALUES (910, 509, 'DEPOSIT', 15000.00, '2026-01-28');

INSERT INTO transactions VALUES (911, 510, 'WITHDRAWAL', 50.00, '2026-02-01');

INSERT INTO transactions VALUES (912, 511, 'DEPOSIT', 800.00, '2026-02-03');

INSERT INTO transactions VALUES (913, 512, 'DEPOSIT', 2000.00, '2026-02-05');

INSERT INTO transactions VALUES (914, 513, 'WITHDRAWAL', 300.00, '2026-02-08');

INSERT INTO transactions VALUES (915, 515, 'DEPOSIT', 4000.00, '2026-02-10');

INSERT INTO transactions VALUES (916, 516, 'DEPOSIT', 8000.00, '2026-02-12');

INSERT INTO transactions VALUES (917, 517, 'WITHDRAWAL', 500.00, '2026-02-15');

INSERT INTO transactions VALUES (918, 518, 'DEPOSIT', 1200.00, '2026-02-18');

INSERT INTO transactions VALUES (919, 519, 'DEPOSIT', 20000.00, '2026-02-20');

INSERT INTO transactions VALUES (920, 520, 'DEPOSIT', 25000.00, '2026-02-22');

UPDATE customers SET credit_score = 760 WHERE id = 101;

UPDATE accounts SET balance = 5000.00 WHERE id = 501;

UPDATE accounts SET balance = 2000.00 WHERE id = 503;

DELETE FROM customers WHERE id = 113;

DELETE FROM accounts WHERE id = 514;
