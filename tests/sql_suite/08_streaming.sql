DROP DATABASE IF EXISTS test_streaming;

CREATE DATABASE test_streaming;

\c test_streaming;

DROP TABLE IF EXISTS watch_history;

DROP TABLE IF EXISTS subscribers;

DROP TABLE IF EXISTS episodes;

DROP TABLE IF EXISTS shows;

CREATE TABLE shows (
    id INT PRIMARY KEY,
    title VARCHAR(50),
    genre VARCHAR(30),
    release_year INT
);

CREATE TABLE episodes (
    id INT PRIMARY KEY,
    show_id INT,
    season_num INT,
    episode_num INT,
    title VARCHAR(50),
    duration_min INT
);

CREATE TABLE subscribers (
    id INT PRIMARY KEY,
    username VARCHAR(50),
    plan VARCHAR(20),
    monthly_fee FLOAT
);

CREATE TABLE watch_history (
    id INT PRIMARY KEY,
    subscriber_id INT,
    episode_id INT,
    watch_date VARCHAR(20),
    completed BOOL
);

INSERT INTO shows VALUES (1, 'Stranger Things', 'Sci-Fi', 2016);

INSERT INTO shows VALUES (2, 'The Crown', 'Drama', 2016);

INSERT INTO shows VALUES (3, 'Breaking Bad', 'Crime', 2008);

INSERT INTO shows VALUES (4, 'The Office', 'Comedy', 2005);

INSERT INTO shows VALUES (5, 'Planet Earth', 'Documentary', 2006);

INSERT INTO episodes VALUES (101, 1, 1, 1, 'Chapter One: The Vanishing', 48);

INSERT INTO episodes VALUES (102, 1, 1, 2, 'Chapter Two: The Weirdo', 55);

INSERT INTO episodes VALUES (103, 1, 1, 3, 'Chapter Three: Holly Jolly', 51);

INSERT INTO episodes VALUES (104, 2, 1, 1, 'Wolferton Splash', 57);

INSERT INTO episodes VALUES (105, 2, 1, 2, 'Hyde Park Corner', 61);

INSERT INTO episodes VALUES (106, 3, 1, 1, 'Pilot', 58);

INSERT INTO episodes VALUES (107, 3, 1, 2, 'Cats in the Bag...', 48);

INSERT INTO episodes VALUES (108, 4, 1, 1, 'Pilot', 22);

INSERT INTO episodes VALUES (109, 4, 1, 2, 'Diversity Day', 22);

INSERT INTO episodes VALUES (110, 4, 1, 3, 'Health Care', 22);

INSERT INTO episodes VALUES (111, 5, 1, 1, 'From Pole to Pole', 50);

INSERT INTO episodes VALUES (112, 5, 1, 2, 'Mountains', 50);

INSERT INTO episodes VALUES (113, 1, 2, 1, 'MADMAX', 48);

INSERT INTO episodes VALUES (114, 2, 2, 1, 'Misadventure', 56);

INSERT INTO episodes VALUES (115, 3, 2, 1, 'Seven Thirty-Seven', 47);

INSERT INTO episodes VALUES (116, 4, 2, 1, 'The Dundies', 22);

INSERT INTO episodes VALUES (117, 4, 2, 2, 'Sexual Harassment', 22);

INSERT INTO episodes VALUES (118, 5, 1, 3, 'Fresh Water', 50);

INSERT INTO episodes VALUES (119, 1, 2, 2, 'Trick or Treat, Freak', 56);

INSERT INTO episodes VALUES (120, 3, 2, 2, 'Grill', 48);

INSERT INTO subscribers VALUES (501, 'alex99', 'BASIC', 9.99);

INSERT INTO subscribers VALUES (502, 'bella_s', 'STANDARD', 15.49);

INSERT INTO subscribers VALUES (503, 'charlie_dev', 'PREMIUM', 19.99);

INSERT INTO subscribers VALUES (504, 'david_k', 'STANDARD', 15.49);

INSERT INTO subscribers VALUES (505, 'emily_r', 'BASIC', 9.99);

INSERT INTO subscribers VALUES (506, 'frank_t', 'PREMIUM', 19.99);

INSERT INTO subscribers VALUES (507, 'grace_m', 'STANDARD', 15.49);

INSERT INTO subscribers VALUES (508, 'harry_p', 'BASIC', 9.99);

INSERT INTO subscribers VALUES (509, 'iris_w', 'PREMIUM', 19.99);

INSERT INTO subscribers VALUES (510, 'jack_s', 'STANDARD', 15.49);

INSERT INTO subscribers VALUES (511, 'karen_g', 'BASIC', 9.99);

INSERT INTO subscribers VALUES (512, 'liam_n', 'PREMIUM', 19.99);

INSERT INTO subscribers VALUES (513, 'mia_h', 'STANDARD', 15.49);

INSERT INTO subscribers VALUES (514, 'noah_b', 'BASIC', 9.99);

INSERT INTO subscribers VALUES (515, 'olivia_w', 'PREMIUM', 19.99);

INSERT INTO subscribers VALUES (516, 'paul_a', 'STANDARD', 15.49);

INSERT INTO subscribers VALUES (517, 'quinn_f', 'BASIC', 9.99);

INSERT INTO subscribers VALUES (518, 'ryan_g', 'PREMIUM', 19.99);

INSERT INTO subscribers VALUES (519, 'sophia_l', 'STANDARD', 15.49);

INSERT INTO subscribers VALUES (520, 'tom_h', 'BASIC', 9.99);

INSERT INTO watch_history VALUES (901, 501, 101, '2026-01-10', true);

INSERT INTO watch_history VALUES (902, 501, 102, '2026-01-10', true);

INSERT INTO watch_history VALUES (903, 502, 104, '2026-01-12', true);

INSERT INTO watch_history VALUES (904, 503, 106, '2026-01-15', true);

INSERT INTO watch_history VALUES (905, 504, 108, '2026-01-18', true);

INSERT INTO watch_history VALUES (906, 505, 111, '2026-01-20', false);

INSERT INTO watch_history VALUES (907, 506, 101, '2026-01-22', true);

INSERT INTO watch_history VALUES (908, 507, 105, '2026-01-25', true);

INSERT INTO watch_history VALUES (909, 508, 108, '2026-01-28', false);

INSERT INTO watch_history VALUES (910, 509, 106, '2026-02-01', true);

INSERT INTO watch_history VALUES (911, 510, 109, '2026-02-03', true);

INSERT INTO watch_history VALUES (912, 511, 112, '2026-02-05', true);

INSERT INTO watch_history VALUES (913, 512, 103, '2026-02-08', true);

INSERT INTO watch_history VALUES (914, 513, 107, '2026-02-10', true);

INSERT INTO watch_history VALUES (915, 514, 110, '2026-02-12', false);

INSERT INTO watch_history VALUES (916, 515, 113, '2026-02-15', true);

INSERT INTO watch_history VALUES (917, 516, 116, '2026-02-18', true);

INSERT INTO watch_history VALUES (918, 517, 118, '2026-02-20', true);

INSERT INTO watch_history VALUES (919, 518, 119, '2026-02-22', true);

INSERT INTO watch_history VALUES (920, 519, 120, '2026-02-25', false);

UPDATE subscribers SET plan = 'PREMIUM' WHERE id = 501;

UPDATE subscribers SET monthly_fee = 19.99 WHERE id = 501;

UPDATE watch_history SET completed = true WHERE id = 906;

DELETE FROM subscribers WHERE id = 520;

DELETE FROM watch_history WHERE id = 920;

SELECT id, title, genre, release_year FROM shows ORDER BY id ASC;

SELECT id, show_id, title, duration_min FROM episodes WHERE duration_min >= 50 ORDER BY duration_min DESC;

SELECT id, username, plan, monthly_fee FROM subscribers WHERE plan = 'PREMIUM' ORDER BY id ASC;

SELECT id, subscriber_id, episode_id, completed FROM watch_history WHERE completed = true ORDER BY id ASC;

SELECT count(*) FROM subscribers;

SELECT count(*) FROM episodes WHERE show_id = 4;

SELECT min(duration_min), max(duration_min), avg(duration_min) FROM episodes;

SELECT sum(monthly_fee) FROM subscribers;

SELECT plan, count(*) FROM subscribers GROUP BY plan ORDER BY plan ASC;

SELECT genre, count(*) FROM shows GROUP BY genre ORDER BY genre ASC;

SELECT episodes.id, episodes.title, shows.title FROM episodes INNER JOIN shows ON episodes.show_id = shows.id ORDER BY episodes.id ASC;

SELECT watch_history.id, subscribers.username, episodes.title FROM watch_history INNER JOIN subscribers ON watch_history.subscriber_id = subscribers.id INNER JOIN episodes ON watch_history.episode_id = episodes.id ORDER BY watch_history.id ASC;

SELECT id, username, monthly_fee FROM subscribers WHERE monthly_fee > 10.00 AND plan = 'STANDARD' ORDER BY id ASC;

SELECT id, title, duration_min FROM episodes WHERE show_id = 1 OR show_id = 3 ORDER BY id ASC;

SELECT id, username, monthly_fee FROM subscribers ORDER BY monthly_fee DESC LIMIT 5;

SELECT id, username, monthly_fee FROM subscribers ORDER BY monthly_fee DESC LIMIT 5 OFFSET 5;
