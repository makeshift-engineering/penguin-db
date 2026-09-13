DROP DATABASE IF EXISTS test_healthcare;

CREATE DATABASE test_healthcare;

\c test_healthcare;

DROP TABLE IF EXISTS prescriptions;

DROP TABLE IF EXISTS appointments;

DROP TABLE IF EXISTS patients;

DROP TABLE IF EXISTS doctors;

CREATE TABLE doctors (
    id INT PRIMARY KEY,
    name VARCHAR(50),
    specialty VARCHAR(50),
    years_exp INT
);

CREATE TABLE patients (
    id INT PRIMARY KEY,
    doctor_id INT,
    name VARCHAR(50),
    age INT,
    blood_type VARCHAR(5)
);

CREATE TABLE appointments (
    id INT PRIMARY KEY,
    patient_id INT,
    doctor_id INT,
    appt_date VARCHAR(20),
    status VARCHAR(20)
);

CREATE TABLE prescriptions (
    id INT PRIMARY KEY,
    patient_id INT,
    medication VARCHAR(50),
    dosage VARCHAR(20)
);

INSERT INTO doctors VALUES (1, 'Dr. Gregory House', 'Diagnostics', 25);

INSERT INTO doctors VALUES (2, 'Dr. Meredith Grey', 'General Surgery', 18);

INSERT INTO doctors VALUES (3, 'Dr. John Watson', 'General Medicine', 15);

INSERT INTO doctors VALUES (4, 'Dr. Leonard McCoy', 'Cardiology', 22);

INSERT INTO doctors VALUES (5, 'Dr. Stephen Strange', 'Neurosurgery', 20);

INSERT INTO patients VALUES (101, 1, 'Alice Smith', 32, 'A+');

INSERT INTO patients VALUES (102, 1, 'Bob Jones', 45, 'O+');

INSERT INTO patients VALUES (103, 1, 'Charlie Brown', 58, 'B-');

INSERT INTO patients VALUES (104, 2, 'Diana Prince', 29, 'AB+');

INSERT INTO patients VALUES (105, 2, 'Evan Wright', 63, 'O-');

INSERT INTO patients VALUES (106, 2, 'Fiona Gallagher', 24, 'A-');

INSERT INTO patients VALUES (107, 3, 'George Clark', 51, 'A+');

INSERT INTO patients VALUES (108, 3, 'Hannah Abbott', 37, 'B+');

INSERT INTO patients VALUES (109, 3, 'Ian Malcolm', 42, 'O+');

INSERT INTO patients VALUES (110, 4, 'Julia Roberts', 65, 'A+');

INSERT INTO patients VALUES (111, 4, 'Kevin Bacon', 54, 'O-');

INSERT INTO patients VALUES (112, 4, 'Laura Croft', 31, 'AB-');

INSERT INTO patients VALUES (113, 5, 'Michael Scott', 49, 'B+');

INSERT INTO patients VALUES (114, 5, 'Nina Williams', 27, 'A+');

INSERT INTO patients VALUES (115, 1, 'Oscar Martinez', 38, 'O+');

INSERT INTO patients VALUES (116, 2, 'Pam Beesly', 33, 'A-');

INSERT INTO patients VALUES (117, 3, 'Quentin Tarantino', 61, 'O+');

INSERT INTO patients VALUES (118, 4, 'Rachel Green', 30, 'B+');

INSERT INTO patients VALUES (119, 5, 'Steve Rogers', 72, 'O+');

INSERT INTO patients VALUES (120, 5, 'Tony Stark', 48, 'A+');

INSERT INTO appointments VALUES (501, 101, 1, '2026-01-10', 'COMPLETED');

INSERT INTO appointments VALUES (502, 102, 1, '2026-01-11', 'COMPLETED');

INSERT INTO appointments VALUES (503, 104, 2, '2026-01-15', 'SCHEDULED');

INSERT INTO appointments VALUES (504, 105, 2, '2026-01-18', 'COMPLETED');

INSERT INTO appointments VALUES (505, 107, 3, '2026-01-20', 'CANCELLED');

INSERT INTO appointments VALUES (506, 108, 3, '2026-01-22', 'COMPLETED');

INSERT INTO appointments VALUES (507, 110, 4, '2026-01-25', 'COMPLETED');

INSERT INTO appointments VALUES (508, 111, 4, '2026-01-28', 'SCHEDULED');

INSERT INTO appointments VALUES (509, 113, 5, '2026-02-01', 'COMPLETED');

INSERT INTO appointments VALUES (510, 114, 5, '2026-02-03', 'COMPLETED');

INSERT INTO appointments VALUES (511, 115, 1, '2026-02-05', 'SCHEDULED');

INSERT INTO appointments VALUES (512, 116, 2, '2026-02-08', 'COMPLETED');

INSERT INTO appointments VALUES (513, 117, 3, '2026-02-10', 'COMPLETED');

INSERT INTO appointments VALUES (514, 118, 4, '2026-02-12', 'SCHEDULED');

INSERT INTO appointments VALUES (515, 119, 5, '2026-02-15', 'COMPLETED');

INSERT INTO appointments VALUES (516, 120, 5, '2026-02-18', 'COMPLETED');

INSERT INTO appointments VALUES (517, 103, 1, '2026-02-20', 'SCHEDULED');

INSERT INTO appointments VALUES (518, 106, 2, '2026-02-22', 'COMPLETED');

INSERT INTO appointments VALUES (519, 109, 3, '2026-02-25', 'COMPLETED');

INSERT INTO appointments VALUES (520, 112, 4, '2026-02-28', 'SCHEDULED');

INSERT INTO prescriptions VALUES (801, 101, 'Amoxicillin', '500mg');

INSERT INTO prescriptions VALUES (802, 102, 'Lisinopril', '10mg');

INSERT INTO prescriptions VALUES (803, 104, 'Ibuprofen', '400mg');

INSERT INTO prescriptions VALUES (804, 105, 'Metformin', '850mg');

INSERT INTO prescriptions VALUES (805, 108, 'Omeprazole', '20mg');

INSERT INTO prescriptions VALUES (806, 110, 'Atorvastatin', '40mg');

INSERT INTO prescriptions VALUES (807, 113, 'Gabapentin', '300mg');

INSERT INTO prescriptions VALUES (808, 114, 'Amoxicillin', '250mg');

INSERT INTO prescriptions VALUES (809, 116, 'Levothyroxine', '50mcg');

INSERT INTO prescriptions VALUES (810, 117, 'Amlodipine', '5mg');

INSERT INTO prescriptions VALUES (811, 119, 'Metoprolol', '50mg');

INSERT INTO prescriptions VALUES (812, 120, 'Aspirin', '81mg');

INSERT INTO prescriptions VALUES (813, 103, 'Prednisone', '10mg');

INSERT INTO prescriptions VALUES (814, 106, 'Cetirizine', '10mg');

INSERT INTO prescriptions VALUES (815, 109, 'Albuterol', '90mcg');

INSERT INTO prescriptions VALUES (816, 112, 'Losartan', '50mg');

INSERT INTO prescriptions VALUES (817, 115, 'Sertraline', '50mg');

INSERT INTO prescriptions VALUES (818, 118, 'Hydrochlorothiazide', '25mg');

INSERT INTO prescriptions VALUES (819, 107, 'Vitamin D3', '2000IU');

INSERT INTO prescriptions VALUES (820, 111, 'Simvastatin', '20mg');

UPDATE patients SET age = 33 WHERE id = 101;

UPDATE doctors SET years_exp = 26 WHERE id = 1;

UPDATE appointments SET status = 'COMPLETED' WHERE id = 503;

DELETE FROM patients WHERE id = 107;

DELETE FROM prescriptions WHERE id = 819;

SELECT id, name, specialty, years_exp FROM doctors ORDER BY years_exp DESC;

SELECT id, name, age, blood_type FROM patients WHERE age >= 50 ORDER BY age DESC;

SELECT id, patient_id, doctor_id, status FROM appointments WHERE status = 'COMPLETED' ORDER BY id ASC;

SELECT id, patient_id, medication, dosage FROM prescriptions WHERE medication = 'Amoxicillin' ORDER BY id ASC;

SELECT count(*) FROM patients;

SELECT count(*) FROM appointments WHERE status = 'SCHEDULED';

SELECT min(age), max(age), avg(age) FROM patients;

SELECT doctor_id, count(*) FROM patients GROUP BY doctor_id ORDER BY doctor_id ASC;

SELECT blood_type, count(*) FROM patients GROUP BY blood_type ORDER BY blood_type ASC;

SELECT patients.id, patients.name, doctors.name FROM patients INNER JOIN doctors ON patients.doctor_id = doctors.id ORDER BY patients.id ASC;

SELECT appointments.id, patients.name, doctors.name, appointments.status FROM appointments INNER JOIN patients ON appointments.patient_id = patients.id INNER JOIN doctors ON appointments.doctor_id = doctors.id ORDER BY appointments.id ASC;

SELECT patients.id, patients.name, prescriptions.medication FROM patients INNER JOIN prescriptions ON patients.id = prescriptions.patient_id ORDER BY patients.id ASC;

SELECT id, name, age FROM patients WHERE age > 30 AND blood_type = 'A+' ORDER BY id ASC;

SELECT id, name, age FROM patients WHERE blood_type = 'O+' OR blood_type = 'O-' ORDER BY id ASC;

SELECT id, name, age FROM patients ORDER BY age DESC LIMIT 5;

SELECT id, name, age FROM patients ORDER BY age DESC LIMIT 5 OFFSET 5;
