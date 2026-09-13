DROP DATABASE IF EXISTS test_hr_payroll;

CREATE DATABASE test_hr_payroll;

\c test_hr_payroll;

DROP TABLE IF EXISTS projects;

DROP TABLE IF EXISTS salaries;

DROP TABLE IF EXISTS employees;

DROP TABLE IF EXISTS departments;

CREATE TABLE departments (
    id INT PRIMARY KEY,
    dept_name VARCHAR(50),
    location VARCHAR(50),
    budget FLOAT
);

CREATE TABLE employees (
    id INT PRIMARY KEY,
    dept_id INT,
    first_name VARCHAR(50),
    last_name VARCHAR(50),
    job_title VARCHAR(50),
    hire_date VARCHAR(20)
);

CREATE TABLE salaries (
    id INT PRIMARY KEY,
    emp_id INT,
    salary FLOAT,
    bonus FLOAT
);

CREATE TABLE projects (
    id INT PRIMARY KEY,
    dept_id INT,
    project_name VARCHAR(50),
    status VARCHAR(20)
);

INSERT INTO departments VALUES (1, 'Engineering', 'New York', 500000.00);

INSERT INTO departments VALUES (2, 'Sales', 'Chicago', 300000.00);

INSERT INTO departments VALUES (3, 'Marketing', 'San Francisco', 250000.00);

INSERT INTO departments VALUES (4, 'Human Resources', 'New York', 150000.00);

INSERT INTO employees VALUES (101, 1, 'John', 'Doe', 'Software Engineer', '2021-03-15');

INSERT INTO employees VALUES (102, 1, 'Jane', 'Smith', 'Senior Developer', '2019-06-01');

INSERT INTO employees VALUES (103, 1, 'Robert', 'Johnson', 'DevOps Lead', '2020-01-10');

INSERT INTO employees VALUES (104, 2, 'Emily', 'Davis', 'Account Executive', '2022-04-18');

INSERT INTO employees VALUES (105, 2, 'Michael', 'Brown', 'Sales Manager', '2018-11-05');

INSERT INTO employees VALUES (106, 3, 'Sarah', 'Wilson', 'Content Specialist', '2023-02-01');

INSERT INTO employees VALUES (107, 3, 'David', 'Taylor', 'Marketing Director', '2017-08-20');

INSERT INTO employees VALUES (108, 4, 'Jessica', 'Anderson', 'HR Specialist', '2021-09-12');

INSERT INTO employees VALUES (109, 1, 'James', 'Thomas', 'QA Engineer', '2022-08-15');

INSERT INTO employees VALUES (110, 1, 'Amanda', 'Jackson', 'Frontend Developer', '2023-01-10');

INSERT INTO employees VALUES (111, 2, 'Daniel', 'White', 'Sales Rep', '2023-05-20');

INSERT INTO employees VALUES (112, 2, 'Megan', 'Harris', 'Sales Rep', '2022-11-14');

INSERT INTO employees VALUES (113, 3, 'Chris', 'Martin', 'SEO Specialist', '2021-07-19');

INSERT INTO employees VALUES (114, 3, 'Laura', 'Thompson', 'Social Media Lead', '2020-10-05');

INSERT INTO employees VALUES (115, 4, 'Brian', 'Garcia', 'Recruiter', '2022-03-30');

INSERT INTO employees VALUES (116, 1, 'Nicole', 'Martinez', 'Backend Engineer', '2021-12-01');

INSERT INTO employees VALUES (117, 1, 'Andrew', 'Robinson', 'System Architect', '2016-04-15');

INSERT INTO employees VALUES (118, 2, 'Samantha', 'Clark', 'Account Executive', '2023-06-01');

INSERT INTO employees VALUES (119, 3, 'Tyler', 'Rodriguez', 'Graphic Designer', '2022-09-01');

INSERT INTO employees VALUES (120, 4, 'Rachel', 'Lewis', 'HR Director', '2015-02-10');

INSERT INTO salaries VALUES (1, 101, 95000.00, 5000.00);

INSERT INTO salaries VALUES (2, 102, 130000.00, 10000.00);

INSERT INTO salaries VALUES (3, 103, 125000.00, 8000.00);

INSERT INTO salaries VALUES (4, 104, 75000.00, 15000.00);

INSERT INTO salaries VALUES (5, 105, 110000.00, 20000.00);

INSERT INTO salaries VALUES (6, 106, 62000.00, 3000.00);

INSERT INTO salaries VALUES (7, 107, 140000.00, 12000.00);

INSERT INTO salaries VALUES (8, 108, 68000.00, 4000.00);

INSERT INTO salaries VALUES (9, 109, 82000.00, 4500.00);

INSERT INTO salaries VALUES (10, 110, 90000.00, 5000.00);

INSERT INTO salaries VALUES (11, 111, 58000.00, 10000.00);

INSERT INTO salaries VALUES (12, 112, 60000.00, 11000.00);

INSERT INTO salaries VALUES (13, 113, 65000.00, 3500.00);

INSERT INTO salaries VALUES (14, 114, 72000.00, 5000.00);

INSERT INTO salaries VALUES (15, 115, 64000.00, 3000.00);

INSERT INTO salaries VALUES (16, 116, 98000.00, 6000.00);

INSERT INTO salaries VALUES (17, 117, 160000.00, 15000.00);

INSERT INTO salaries VALUES (18, 118, 59000.00, 9500.00);

INSERT INTO salaries VALUES (19, 119, 61000.00, 3000.00);

INSERT INTO salaries VALUES (20, 120, 135000.00, 10000.00);

INSERT INTO projects VALUES (1, 1, 'Cloud Migration', 'IN_PROGRESS');

INSERT INTO projects VALUES (2, 1, 'API Gateway v2', 'COMPLETED');

INSERT INTO projects VALUES (3, 2, 'Q3 Sales Blitz', 'COMPLETED');

INSERT INTO projects VALUES (4, 3, 'Brand Refresh', 'IN_PROGRESS');

INSERT INTO projects VALUES (5, 4, 'Workplace Wellness', 'PLANNING');

INSERT INTO projects VALUES (6, 1, 'Data Warehouse', 'PLANNING');

INSERT INTO projects VALUES (7, 2, 'Enterprise Expansion', 'IN_PROGRESS');

INSERT INTO projects VALUES (8, 3, 'SEO Overhaul', 'COMPLETED');

INSERT INTO projects VALUES (9, 4, 'Recruitment Drive', 'IN_PROGRESS');

INSERT INTO projects VALUES (10, 1, 'Security Audit', 'COMPLETED');

UPDATE salaries SET salary = 100000.00 WHERE emp_id = 101;

UPDATE departments SET budget = 550000.00 WHERE id = 1;

UPDATE projects SET status = 'COMPLETED' WHERE id = 1;

DELETE FROM employees WHERE id = 119;

DELETE FROM salaries WHERE emp_id = 119;

SELECT id, dept_name, location, budget FROM departments ORDER BY id ASC;

SELECT id, first_name, last_name, job_title FROM employees WHERE dept_id = 1 ORDER BY id ASC;

SELECT emp_id, salary, bonus FROM salaries WHERE salary >= 100000.00 ORDER BY salary DESC;

SELECT id, project_name, status FROM projects WHERE status = 'IN_PROGRESS' ORDER BY id ASC;

SELECT count(*) FROM employees;

SELECT dept_id, count(*) FROM employees GROUP BY dept_id ORDER BY dept_id ASC;

SELECT min(salary), max(salary), avg(salary) FROM salaries;

SELECT sum(budget) FROM departments;

SELECT emp_id, salary + bonus FROM salaries ORDER BY emp_id ASC LIMIT 10;

SELECT employees.id, employees.first_name, employees.last_name, departments.dept_name FROM employees INNER JOIN departments ON employees.dept_id = departments.id ORDER BY employees.id ASC;

SELECT employees.id, employees.first_name, salaries.salary, salaries.bonus FROM employees INNER JOIN salaries ON employees.id = salaries.emp_id ORDER BY employees.id ASC;

SELECT departments.dept_name, projects.project_name, projects.status FROM departments INNER JOIN projects ON departments.id = projects.dept_id ORDER BY departments.id ASC;

SELECT employees.first_name, employees.last_name FROM employees WHERE job_title = 'Sales Rep' OR job_title = 'Account Executive' ORDER BY employees.id ASC;

SELECT id, first_name, last_name FROM employees WHERE NOT (dept_id = 1) ORDER BY id ASC;

SELECT id, first_name, hire_date FROM employees ORDER BY hire_date ASC LIMIT 5;

SELECT id, first_name, hire_date FROM employees ORDER BY hire_date ASC LIMIT 5 OFFSET 5;
