DROP DATABASE IF EXISTS test_academics;

CREATE DATABASE test_academics;

\c test_academics;

DROP TABLE IF EXISTS enrollments;

DROP TABLE IF EXISTS students;

DROP TABLE IF EXISTS courses;

DROP TABLE IF EXISTS instructors;

CREATE TABLE instructors (
    id INT PRIMARY KEY,
    name VARCHAR(50),
    department VARCHAR(50),
    title VARCHAR(30)
);

CREATE TABLE courses (
    id INT PRIMARY KEY,
    instructor_id INT,
    course_name VARCHAR(50),
    credits INT
);

CREATE TABLE students (
    id INT PRIMARY KEY,
    name VARCHAR(50),
    gpa FLOAT,
    enrollment_year INT
);

CREATE TABLE enrollments (
    id INT PRIMARY KEY,
    student_id INT,
    course_id INT,
    grade VARCHAR(5),
    score FLOAT
);

INSERT INTO instructors VALUES (1, 'Dr. Alan Turing', 'Computer Science', 'Professor');

INSERT INTO instructors VALUES (2, 'Dr. Marie Curie', 'Physics', 'Professor');

INSERT INTO instructors VALUES (3, 'Dr. Carl Gauss', 'Mathematics', 'Associate Professor');

INSERT INTO instructors VALUES (4, 'Dr. Ada Lovelace', 'Computer Science', 'Assistant Professor');

INSERT INTO courses VALUES (101, 1, 'Data Structures', 4);

INSERT INTO courses VALUES (102, 1, 'Algorithms', 4);

INSERT INTO courses VALUES (103, 2, 'Quantum Mechanics', 3);

INSERT INTO courses VALUES (104, 3, 'Linear Algebra', 3);

INSERT INTO courses VALUES (105, 3, 'Calculus III', 4);

INSERT INTO courses VALUES (106, 4, 'Artificial Intelligence', 4);

INSERT INTO students VALUES (1, 'Alice Walker', 3.85, 2023);

INSERT INTO students VALUES (2, 'Bob Builder', 3.20, 2022);

INSERT INTO students VALUES (3, 'Charlie Chaplin', 2.90, 2024);

INSERT INTO students VALUES (4, 'David Beckham', 3.50, 2023);

INSERT INTO students VALUES (5, 'Eva Green', 3.95, 2021);

INSERT INTO students VALUES (6, 'Frank Sinatra', 3.10, 2022);

INSERT INTO students VALUES (7, 'Grace Hopper', 4.00, 2021);

INSERT INTO students VALUES (8, 'Harry Potter', 3.40, 2023);

INSERT INTO students VALUES (9, 'Iris West', 3.75, 2022);

INSERT INTO students VALUES (10, 'Jack Sparrow', 2.80, 2024);

INSERT INTO students VALUES (11, 'Katniss Everdeen', 3.65, 2023);

INSERT INTO students VALUES (12, 'Luke Skywalker', 3.30, 2022);

INSERT INTO students VALUES (13, 'Mary Jane', 3.80, 2021);

INSERT INTO students VALUES (14, 'Neville Longbottom', 3.15, 2024);

INSERT INTO students VALUES (15, 'Oliver Queen', 2.95, 2023);

INSERT INTO students VALUES (16, 'Peter Parker', 3.90, 2022);

INSERT INTO students VALUES (17, 'Quinn Fabray', 3.45, 2023);

INSERT INTO students VALUES (18, 'Ron Weasley', 2.85, 2024);

INSERT INTO students VALUES (19, 'Sarah Connor', 3.70, 2021);

INSERT INTO students VALUES (20, 'Tom Riddle', 3.98, 2022);

INSERT INTO enrollments VALUES (501, 1, 101, 'A', 95.0);

INSERT INTO enrollments VALUES (502, 1, 102, 'A', 92.5);

INSERT INTO enrollments VALUES (503, 2, 101, 'B', 84.0);

INSERT INTO enrollments VALUES (504, 2, 104, 'B', 82.0);

INSERT INTO enrollments VALUES (505, 3, 105, 'C', 75.0);

INSERT INTO enrollments VALUES (506, 4, 101, 'A', 90.0);

INSERT INTO enrollments VALUES (507, 4, 106, 'B', 88.0);

INSERT INTO enrollments VALUES (508, 5, 102, 'A', 98.0);

INSERT INTO enrollments VALUES (509, 5, 106, 'A', 96.0);

INSERT INTO enrollments VALUES (510, 6, 104, 'B', 81.5);

INSERT INTO enrollments VALUES (511, 7, 101, 'A', 100.0);

INSERT INTO enrollments VALUES (512, 7, 102, 'A', 99.0);

INSERT INTO enrollments VALUES (513, 8, 103, 'B', 85.0);

INSERT INTO enrollments VALUES (514, 9, 104, 'A', 91.0);

INSERT INTO enrollments VALUES (515, 10, 105, 'C', 72.0);

INSERT INTO enrollments VALUES (516, 11, 106, 'A', 93.0);

INSERT INTO enrollments VALUES (517, 12, 101, 'B', 83.0);

INSERT INTO enrollments VALUES (518, 13, 103, 'A', 94.0);

INSERT INTO enrollments VALUES (519, 16, 102, 'A', 97.0);

INSERT INTO enrollments VALUES (520, 20, 106, 'A', 99.5);

UPDATE students SET gpa = 3.90 WHERE id = 1;

UPDATE enrollments SET score = 86.0 WHERE id = 503;

DELETE FROM students WHERE id = 18;

DELETE FROM enrollments WHERE student_id = 18;

SELECT id, name, gpa FROM students ORDER BY id ASC;

SELECT id, name, gpa FROM students WHERE gpa >= 3.50 ORDER BY gpa DESC;

SELECT id, course_name, credits FROM courses WHERE credits = 4 ORDER BY id ASC;

SELECT id, name, department FROM instructors ORDER BY id ASC;

SELECT count(*) FROM students;

SELECT count(*) FROM students WHERE enrollment_year = 2023;

SELECT min(gpa), max(gpa), avg(gpa) FROM students;

SELECT avg(score) FROM enrollments;

SELECT course_id, count(*) FROM enrollments GROUP BY course_id ORDER BY course_id ASC;

SELECT grade, count(*) FROM enrollments GROUP BY grade ORDER BY grade ASC;

SELECT students.id, students.name, enrollments.course_id, enrollments.grade FROM students INNER JOIN enrollments ON students.id = enrollments.student_id ORDER BY students.id ASC;

SELECT instructors.name, courses.course_name FROM instructors INNER JOIN courses ON instructors.id = courses.instructor_id ORDER BY instructors.id ASC;

SELECT students.name, students.gpa FROM students WHERE gpa > 3.00 AND enrollment_year >= 2022 ORDER BY gpa DESC;

SELECT id, name FROM students WHERE enrollment_year = 2021 OR enrollment_year = 2024 ORDER BY id ASC;

SELECT id, name, gpa FROM students ORDER BY gpa DESC LIMIT 5;

SELECT id, name, gpa FROM students ORDER BY gpa DESC LIMIT 5 OFFSET 5;
