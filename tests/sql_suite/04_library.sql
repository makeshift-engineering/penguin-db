DROP DATABASE IF EXISTS test_library;

CREATE DATABASE test_library;

\c test_library;

DROP TABLE IF EXISTS loans;

DROP TABLE IF EXISTS borrowers;

DROP TABLE IF EXISTS books;

DROP TABLE IF EXISTS authors;

CREATE TABLE authors (
    id INT PRIMARY KEY,
    name VARCHAR(50),
    country VARCHAR(30)
);

CREATE TABLE books (
    id INT PRIMARY KEY,
    author_id INT,
    title VARCHAR(50),
    genre VARCHAR(30),
    pub_year INT
);

CREATE TABLE borrowers (
    id INT PRIMARY KEY,
    name VARCHAR(50),
    card_number VARCHAR(20),
    active BOOL
);

CREATE TABLE loans (
    id INT PRIMARY KEY,
    book_id INT,
    borrower_id INT,
    loan_date VARCHAR(20),
    returned BOOL
);

INSERT INTO authors VALUES (1, 'J.K. Rowling', 'UK');

INSERT INTO authors VALUES (2, 'George R.R. Martin', 'USA');

INSERT INTO authors VALUES (3, 'J.R.R. Tolkien', 'UK');

INSERT INTO authors VALUES (4, 'Agatha Christie', 'UK');

INSERT INTO authors VALUES (5, 'Stephen King', 'USA');

INSERT INTO books VALUES (101, 1, 'Harry Potter 1', 'Fantasy', 1997);

INSERT INTO books VALUES (102, 1, 'Harry Potter 2', 'Fantasy', 1998);

INSERT INTO books VALUES (103, 2, 'A Game of Thrones', 'Fantasy', 1996);

INSERT INTO books VALUES (104, 2, 'A Clash of Kings', 'Fantasy', 1998);

INSERT INTO books VALUES (105, 3, 'The Hobbit', 'Fantasy', 1937);

INSERT INTO books VALUES (106, 3, 'The Fellowship of the Ring', 'Fantasy', 1954);

INSERT INTO books VALUES (107, 4, 'And Then There Were None', 'Mystery', 1939);

INSERT INTO books VALUES (108, 4, 'Murder on the Orient Express', 'Mystery', 1934);

INSERT INTO books VALUES (109, 5, 'The Shining', 'Horror', 1977);

INSERT INTO books VALUES (110, 5, 'It', 'Horror', 1986);

INSERT INTO books VALUES (111, 1, 'Harry Potter 3', 'Fantasy', 1999);

INSERT INTO books VALUES (112, 1, 'Harry Potter 4', 'Fantasy', 2000);

INSERT INTO books VALUES (113, 2, 'A Storm of Swords', 'Fantasy', 2000);

INSERT INTO books VALUES (114, 3, 'The Two Towers', 'Fantasy', 1954);

INSERT INTO books VALUES (115, 3, 'The Return of the King', 'Fantasy', 1955);

INSERT INTO books VALUES (116, 4, 'The ABC Murders', 'Mystery', 1936);

INSERT INTO books VALUES (117, 5, 'Misery', 'Horror', 1987);

INSERT INTO books VALUES (118, 5, 'Carrie', 'Horror', 1974);

INSERT INTO books VALUES (119, 5, 'Pet Sematary', 'Horror', 1983);

INSERT INTO books VALUES (120, 4, 'Death on the Nile', 'Mystery', 1937);

INSERT INTO borrowers VALUES (1, 'Alice Johnson', 'CARD-001', true);

INSERT INTO borrowers VALUES (2, 'Bob Williams', 'CARD-002', true);

INSERT INTO borrowers VALUES (3, 'Charlie Brown', 'CARD-003', false);

INSERT INTO borrowers VALUES (4, 'David Miller', 'CARD-004', true);

INSERT INTO borrowers VALUES (5, 'Emma Davis', 'CARD-005', true);

INSERT INTO borrowers VALUES (6, 'Frank Wilson', 'CARD-006', true);

INSERT INTO borrowers VALUES (7, 'Grace Moore', 'CARD-007', false);

INSERT INTO borrowers VALUES (8, 'Henry Taylor', 'CARD-008', true);

INSERT INTO borrowers VALUES (9, 'Isabella Anderson', 'CARD-009', true);

INSERT INTO borrowers VALUES (10, 'Jack Thomas', 'CARD-010', true);

INSERT INTO borrowers VALUES (11, 'Karen Jackson', 'CARD-011', true);

INSERT INTO borrowers VALUES (12, 'Liam White', 'CARD-012', true);

INSERT INTO borrowers VALUES (13, 'Mia Harris', 'CARD-013', false);

INSERT INTO borrowers VALUES (14, 'Noah Martin', 'CARD-014', true);

INSERT INTO borrowers VALUES (15, 'Olivia Thompson', 'CARD-015', true);

INSERT INTO borrowers VALUES (16, 'Paul Garcia', 'CARD-016', true);

INSERT INTO borrowers VALUES (17, 'Rose Martinez', 'CARD-017', true);

INSERT INTO borrowers VALUES (18, 'Sam Robinson', 'CARD-018', false);

INSERT INTO borrowers VALUES (19, 'Tina Clark', 'CARD-019', true);

INSERT INTO borrowers VALUES (20, 'Victor Rodriguez', 'CARD-020', true);

INSERT INTO loans VALUES (501, 101, 1, '2026-01-05', true);

INSERT INTO loans VALUES (502, 103, 2, '2026-01-10', true);

INSERT INTO loans VALUES (503, 105, 4, '2026-01-12', false);

INSERT INTO loans VALUES (504, 107, 5, '2026-01-15', true);

INSERT INTO loans VALUES (505, 109, 6, '2026-01-18', false);

INSERT INTO loans VALUES (506, 111, 8, '2026-01-20', true);

INSERT INTO loans VALUES (507, 113, 9, '2026-01-22', true);

INSERT INTO loans VALUES (508, 115, 10, '2026-01-25', false);

INSERT INTO loans VALUES (509, 102, 11, '2026-01-28', true);

INSERT INTO loans VALUES (510, 104, 12, '2026-02-01', true);

INSERT INTO loans VALUES (511, 106, 14, '2026-02-03', false);

INSERT INTO loans VALUES (512, 108, 15, '2026-02-05', true);

INSERT INTO loans VALUES (513, 110, 16, '2026-02-08', true);

INSERT INTO loans VALUES (514, 112, 17, '2026-02-10', false);

INSERT INTO loans VALUES (515, 114, 19, '2026-02-12', true);

INSERT INTO loans VALUES (516, 116, 20, '2026-02-15', true);

INSERT INTO loans VALUES (517, 117, 1, '2026-02-18', false);

INSERT INTO loans VALUES (518, 118, 2, '2026-02-20', true);

INSERT INTO loans VALUES (519, 119, 4, '2026-02-22', true);

INSERT INTO loans VALUES (520, 120, 5, '2026-02-25', false);

UPDATE books SET pub_year = 1938 WHERE id = 105;

UPDATE borrowers SET active = true WHERE id = 3;

UPDATE loans SET returned = true WHERE id = 503;

DELETE FROM books WHERE id = 120;

DELETE FROM borrowers WHERE id = 18;

SELECT id, name, country FROM authors ORDER BY id ASC;

SELECT id, title, genre, pub_year FROM books WHERE genre = 'Fantasy' ORDER BY pub_year ASC;

SELECT id, title, pub_year FROM books WHERE pub_year < 1950 ORDER BY pub_year ASC;

SELECT id, name, card_number FROM borrowers WHERE active = true ORDER BY id ASC;

SELECT id, book_id, borrower_id, returned FROM loans WHERE returned = false ORDER BY id ASC;

SELECT count(*) FROM books;

SELECT count(*) FROM books WHERE author_id = 5;

SELECT min(pub_year), max(pub_year) FROM books;

SELECT genre, count(*) FROM books GROUP BY genre ORDER BY genre ASC;

SELECT author_id, count(*) FROM books GROUP BY author_id ORDER BY author_id ASC;

SELECT books.id, books.title, authors.name FROM books INNER JOIN authors ON books.author_id = authors.id ORDER BY books.id ASC;

SELECT loans.id, books.title, borrowers.name FROM loans INNER JOIN books ON loans.book_id = books.id INNER JOIN borrowers ON loans.borrower_id = borrowers.id ORDER BY loans.id ASC;

SELECT id, title FROM books WHERE genre = 'Horror' AND pub_year > 1980 ORDER BY id ASC;

SELECT id, title FROM books WHERE genre = 'Mystery' OR pub_year < 1940 ORDER BY id ASC;

SELECT id, title, pub_year FROM books ORDER BY pub_year ASC LIMIT 5;

SELECT id, title, pub_year FROM books ORDER BY pub_year ASC LIMIT 5 OFFSET 5;
